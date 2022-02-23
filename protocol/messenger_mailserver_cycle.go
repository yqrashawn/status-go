package protocol

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/status-im/status-go/params"
	"github.com/status-im/status-go/services/mailservers"
	"github.com/status-im/status-go/signal"
)

const defaultBackoff = 10 * time.Second

func (m *Messenger) mailserversByFleet(fleet string) []mailservers.Mailserver {
	var items []mailservers.Mailserver
	for _, ms := range mailservers.DefaultMailservers() {
		if ms.Fleet == fleet {
			items = append(items, ms)
		}
	}
	return items
}

type byRTTMs []*mailservers.PingResult

func (s byRTTMs) Len() int {
	return len(s)
}

func (s byRTTMs) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byRTTMs) Less(i, j int) bool {
	return *s[i].RTTMs < *s[j].RTTMs
}

func (m *Messenger) activeMailserverID() ([]byte, error) {
	if m.mailserverCycle.activeMailserver == nil {
		return nil, nil
	}

	return m.mailserverCycle.activeMailserver.IDBytes()
}

func (m *Messenger) StartMailserverCycle() error {

	m.logger.Debug("started mailserver cycle")

	m.mailserverCycle.events = make(chan *p2p.PeerEvent, 20)
	m.mailserverCycle.subscription = m.server.SubscribeEvents(m.mailserverCycle.events)

	go m.checkMailserverConnection()
	go m.updateWakuV1PeerStatus()
	go m.updateWakuV2PeerStatus()
	return nil
}

func (m *Messenger) DisconnectActiveMailserver() {
	m.mailserverCycle.Lock()
	defer m.mailserverCycle.Unlock()
	m.disconnectActiveMailserver()
}

func (m *Messenger) disconnectMailserver() error {
	if m.mailserverCycle.activeMailserver == nil {
		m.logger.Info("no active mailserver")
		return nil
	}
	m.logger.Info("Disconnecting active mailserver", zap.String("nodeID", m.mailserverCycle.activeMailserver.ID))
	pInfo, ok := m.mailserverCycle.peers[m.mailserverCycle.activeMailserver.ID]
	if ok {
		pInfo.status = disconnected
		pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
		m.mailserverCycle.peers[m.mailserverCycle.activeMailserver.ID] = pInfo
	} else {
		m.mailserverCycle.peers[m.mailserverCycle.activeMailserver.ID] = peerStatus{
			status:          disconnected,
			mailserver:      *m.mailserverCycle.activeMailserver,
			canConnectAfter: time.Now().Add(defaultBackoff),
		}
	}

	if m.mailserverCycle.activeMailserver.Version == 2 {
		peerID, err := m.mailserverCycle.activeMailserver.PeerID()
		if err != nil {
			return err
		}
		err = m.transport.DropPeer(string(*peerID))
		if err != nil {
			m.logger.Warn("Could not drop peer")
			return err
		}

	} else {
		node, err := m.mailserverCycle.activeMailserver.Enode()
		if err != nil {
			return err
		}
		m.server.RemovePeer(node)
	}

	m.mailserverCycle.activeMailserver = nil
	return nil
}

func (m *Messenger) disconnectActiveMailserver() {
	err := m.disconnectMailserver()
	if err != nil {
		m.logger.Error("failed to disconnect mailserver", zap.Error(err))
	}
	signal.SendMailserverChanged("", "")
}

func (m *Messenger) cycleMailservers() {
	m.logger.Info("Automatically switching mailserver")

	if m.mailserverCycle.activeMailserver != nil {
		m.disconnectActiveMailserver()
	}

	err := m.findNewMailserver()
	if err != nil {
		m.logger.Error("Error getting new mailserver", zap.Error(err))
	}
}

func poolSize(fleetSize int) int {
	return int(math.Ceil(float64(fleetSize) / 4))
}

func (m *Messenger) getFleet() (string, error) {
	var fleet string
	dbFleet, err := m.settings.GetFleet()
	if err != nil {
		return "", err
	}
	if dbFleet != "" {
		fleet = dbFleet
	} else if m.config.clusterConfig.Fleet != "" {
		fleet = m.config.clusterConfig.Fleet
	} else {
		fleet = params.FleetProd
	}
	return fleet, nil
}

func (m *Messenger) waitUntilMailserverAvailable() error {
	pinnedMailserver, err := m.getPinnedMailserver()
	if err != nil {
		m.logger.Error("Could not obtain the pinned mailserver", zap.Error(err))
		return err
	}
	now := time.Now()
	var canConnectAfters []time.Time
	if pinnedMailserver != nil {
		pInfo, ok := m.mailserverCycle.peers[pinnedMailserver.ID]
		if !ok {
			return nil
		}
		canConnectAfters = append(canConnectAfters, pInfo.canConnectAfter)
	} else {

		allMailservers, err := m.allMailservers()
		if err != nil {
			return err
		}
		for _, node := range allMailservers {
			pInfo, ok := m.mailserverCycle.peers[node.ID]
			// No info about mailserver, mailserver is available
			if !ok {
				return nil
			}
			canConnectAfters = append(canConnectAfters, pInfo.canConnectAfter)

		}
	}
	sort.Slice(canConnectAfters, func(i, j int) bool {
		return canConnectAfters[i].Before(canConnectAfters[j])
	})

	// If the first is before, we can return
	if canConnectAfters[0].Before(now) {
		return nil
	}

	time.Sleep(canConnectAfters[0].Sub(now))

	return nil
}

func (m *Messenger) allMailservers() ([]mailservers.Mailserver, error) {
	// Append user mailservers
	fleet, err := m.getFleet()
	if err != nil {
		return nil, err
	}

	allMailservers := m.mailserversByFleet(fleet)

	customMailservers, err := m.mailservers.Mailservers()
	if err != nil {
		return nil, err
	}

	for _, c := range customMailservers {
		if c.Fleet == fleet {
			allMailservers = append(allMailservers, c)
		}
	}

	return allMailservers, nil
}

func (m *Messenger) findNewMailserver() error {
	pinnedMailserver, err := m.getPinnedMailserver()
	if err != nil {
		m.logger.Error("Could not obtain the pinned mailserver", zap.Error(err))
		return err
	}
	if pinnedMailserver != nil {
		return m.connectToMailserver(*pinnedMailserver)
	}

	// Append user mailservers
	fleet, err := m.getFleet()
	if err != nil {
		return err
	}

	allMailservers := m.mailserversByFleet(fleet)

	customMailservers, err := m.mailservers.Mailservers()
	if err != nil {
		return err
	}

	for _, c := range customMailservers {
		if c.Fleet == fleet {
			allMailservers = append(allMailservers, c)
		}
	}

	var mailserverList []mailservers.Mailserver
	now := time.Now()
	for _, node := range allMailservers {
		pInfo, ok := m.mailserverCycle.peers[node.ID]
		if !ok || pInfo.canConnectAfter.Before(now) {
			mailserverList = append(mailserverList, node)
		}
	}

	m.logger.Info("Finding a new mailserver...")

	var mailserverStr []string
	for _, m := range mailserverList {
		mailserverStr = append(mailserverStr, m.Address)
	}

	if len(mailserverList) == 0 {
		m.logger.Warn("No mailservers available") // Do nothing...
		return nil

	}

	var parseFn func(string) (string, error)
	if mailserverList[0].Version == 2 {
		parseFn = mailservers.MultiAddressToAddress
	} else {
		parseFn = mailservers.EnodeStringToAddr
	}

	pingResult, err := mailservers.DoPing(context.Background(), mailserverStr, 500, parseFn)
	if err != nil {
		return err
	}

	var availableMailservers []*mailservers.PingResult
	for _, result := range pingResult {
		if result.Err != nil {
			continue // The results with error are ignored
		}
		availableMailservers = append(availableMailservers, result)
	}
	sort.Sort(byRTTMs(availableMailservers))

	if len(availableMailservers) == 0 {
		m.logger.Warn("No mailservers available") // Do nothing...
		return nil
	}

	// Picks a random mailserver amongs the ones with the lowest latency
	// The pool size is 1/4 of the mailservers were pinged successfully
	pSize := poolSize(len(availableMailservers) - 1)
	if pSize <= 0 {
		pSize = len(availableMailservers)
	}

	r, err := rand.Int(rand.Reader, big.NewInt(int64(pSize)))
	if err != nil {
		return err
	}

	msPing := availableMailservers[r.Int64()]
	var ms mailservers.Mailserver
	for idx := range mailserverList {
		if msPing.Address == mailserverList[idx].Address {
			ms = mailserverList[idx]
		}
	}
	m.logger.Info("Connecting to mailserver", zap.String("address", ms.Address))
	return m.connectToMailserver(ms)
}

func (m *Messenger) activeMailserverStatus() (connStatus, error) {
	if m.mailserverCycle.activeMailserver == nil {
		return disconnected, errors.New("Active mailserver is not set")
	}

	mailserverID := m.mailserverCycle.activeMailserver.ID

	return m.mailserverCycle.peers[mailserverID].status, nil
}

func (m *Messenger) connectToMailserver(ms mailservers.Mailserver) error {

	m.logger.Info("Connecting to mailserver", zap.Any("peer", ms.ID))
	nodeConnected := false

	m.mailserverCycle.activeMailserver = &ms
	signal.SendMailserverChanged(m.mailserverCycle.activeMailserver.Address, m.mailserverCycle.activeMailserver.ID)

	// Adding a peer and marking it as connected can't be executed sync in WakuV1, because
	// There's a delay between requesting a peer being added, and a signal being
	// received after the peer was added. So we first set the peer status as
	// Connecting and once a peerConnected signal is received, we mark it as
	// Connected
	activeMailserverStatus, err := m.activeMailserverStatus()
	if err != nil {
		return err
	}

	if activeMailserverStatus == connected {
		nodeConnected = true
	} else {
		// Attempt to connect to mailserver by adding it as a peer

		if ms.Version == 2 {
			if err := m.transport.DialPeer(ms.Address); err != nil {
				m.logger.Error("failed to dial", zap.Error(err))
				return err
			}
		} else {
			node, err := ms.Enode()
			if err != nil {
				return err
			}
			m.logger.Info("adding peer")
			m.server.AddTrustedPeer(node)
			m.server.AddPeer(node)
			m.logger.Info("peer addded")
			if err := m.peerStore.Update([]*enode.Node{node}); err != nil {
				return err
			}
		}

		pInfo, ok := m.mailserverCycle.peers[ms.ID]
		if ok {
			pInfo.status = connecting
			pInfo.lastConnectionAttempt = time.Now()
			pInfo.mailserver = ms
			m.mailserverCycle.peers[ms.ID] = pInfo
		} else {
			m.mailserverCycle.peers[ms.ID] = peerStatus{
				status:                connecting,
				mailserver:            ms,
				lastConnectionAttempt: time.Now(),
			}
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	timeout := time.After(60 * time.Second)
	loop := true

	for loop {
		select {
		case <-ticker.C:
			activeMailserverStatus, err := m.activeMailserverStatus()
			if err != nil {
				m.logger.Error("error checking server status", zap.Error(err))
				return err
			}
			m.logger.Info("Checking mailserver status", zap.Any("status", activeMailserverStatus))
			if activeMailserverStatus == connected {
				nodeConnected = true
				loop = false
				ticker.Stop()
				break
			}
		case <-timeout:
			m.logger.Info("stopping timeout")
			ticker.Stop()
			loop = false
		}
	}

	if nodeConnected {
		m.logger.Info("Mailserver available", zap.String("id", ms.ID), zap.Any("ms", m.mailserverCycle.peers[ms.ID]))
		signal.SendMailserverAvailable(m.mailserverCycle.activeMailserver.Address, m.mailserverCycle.activeMailserver.ID)
	}

	return nil
}

func (m *Messenger) getActiveMailserver() *mailservers.Mailserver {
	return m.mailserverCycle.activeMailserver
}

func (m *Messenger) isActiveMailserverAvailable() bool {
	mailserverStatus, err := m.activeMailserverStatus()
	if err != nil {
		return false
	}

	return mailserverStatus == connected
}

func (m *Messenger) mailserverAddressToID(uniqueID string) (string, error) {
	allMailservers, err := m.allMailservers()
	if err != nil {
		return "", err
	}

	for _, ms := range allMailservers {
		if uniqueID == ms.UniqueID() {
			return ms.ID, nil
		}

	}

	return "", nil
}

type ConnectedPeer struct {
	UniqueID string
}

func (m *Messenger) mailserverPeersInfo() []ConnectedPeer {
	var connectedPeers []ConnectedPeer
	for _, connectedPeer := range m.server.PeersInfo() {
		connectedPeers = append(connectedPeers, ConnectedPeer{
			// This is a bit fragile, but should work
			UniqueID: strings.TrimSuffix(connectedPeer.Enode, "?discport=0"),
		})
	}

	return connectedPeers
}

func (m *Messenger) handleMailserverCycleEvent(connectedPeers []ConnectedPeer) error {
	m.logger.Info("CONNECTED PEER", zap.Any("connected", connectedPeers))
	m.logger.Info("PEERS", zap.Any("peers", m.mailserverCycle.peers))

	for pID, pInfo := range m.mailserverCycle.peers {
		if pInfo.status == disconnected {
			continue
		}

		// Removing disconnected

		found := false
		for _, connectedPeer := range connectedPeers {
			id, err := m.mailserverAddressToID(connectedPeer.UniqueID)
			if err != nil {
				m.logger.Error("failed to convert id to hex", zap.Error(err))
				return err
			}

			m.logger.Info("Comparing", zap.String("pID", pID), zap.String("id", id))
			if pID == id {
				found = true
				break
			}
		}
		if !found && (pInfo.status == connected || (pInfo.status == connecting && pInfo.lastConnectionAttempt.Add(8*time.Second).Before(time.Now()))) {
			m.logger.Info("Peer disconnected", zap.String("peer", pID))
			pInfo.status = disconnected
			pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
		}

		m.mailserverCycle.peers[pID] = pInfo
	}

	for _, connectedPeer := range connectedPeers {
		id, err := m.mailserverAddressToID(connectedPeer.UniqueID)
		if err != nil {
			m.logger.Error("failed to convert id to hex", zap.Error(err))
			return err
		}
		if id == "" {
			m.logger.Warn("id not found", zap.String("address", connectedPeer.UniqueID))
			continue
		}
		pInfo, ok := m.mailserverCycle.peers[id]
		if !ok || pInfo.status != connected {
			m.logger.Info("Peer connected", zap.String("peer", connectedPeer.UniqueID))
			pInfo.status = connected
			pInfo.canConnectAfter = time.Now().Add(defaultBackoff)

			if m.mailserverCycle.activeMailserver != nil && id == m.mailserverCycle.activeMailserver.ID {
				m.logger.Info("Mailserver available", zap.String("address", connectedPeer.UniqueID))
				signal.SendMailserverAvailable(m.mailserverCycle.activeMailserver.Address, m.mailserverCycle.activeMailserver.ID)
			}
			m.mailserverCycle.peers[id] = pInfo
		}
	}

	m.logger.Info("PEERS 2", zap.Any("peers", m.mailserverCycle.peers))

	return nil
}

func (m *Messenger) updateWakuV1PeerStatus() {

	if m.transport.WakuVersion() != 1 {
		m.logger.Debug("waku version not 1, returning")
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.logger.Info("mailser ticker event")

			err := m.handleMailserverCycleEvent(m.mailserverPeersInfo())
			if err != nil {
				m.logger.Error("failed to handle mailserver cycle event", zap.Error(err))
				return
			}

		case <-m.mailserverCycle.events:
			m.logger.Info("mailserver cyclce event")

			err := m.handleMailserverCycleEvent(m.mailserverPeersInfo())
			if err != nil {
				m.logger.Error("failed to handle mailserver cycle event", zap.Error(err))
				return
			}
		case <-m.quit:
			close(m.mailserverCycle.events)
			m.mailserverCycle.subscription.Unsubscribe()
			return
		}
	}
}

func (m *Messenger) updateWakuV2PeerStatus() {
	if m.transport.WakuVersion() != 2 {
		m.logger.Debug("waku version not 2, returning")
		return
	}

	connSubscription, err := m.transport.SubscribeToConnStatusChanges()
	if err != nil {
		m.logger.Error("Could not subscribe to connection status changes", zap.Error(err))
	}

	for {
		select {
		case status := <-connSubscription.C:
			var connectedPeers []ConnectedPeer
			for id := range status.Peers {
				connectedPeers = append(connectedPeers, ConnectedPeer{UniqueID: id})
			}
			err := m.handleMailserverCycleEvent(connectedPeers)
			if err != nil {
				m.logger.Error("failed to handle mailserver cycle event", zap.Error(err))
				return
			}

		case <-m.quit:
			close(m.mailserverCycle.events)
			m.mailserverCycle.subscription.Unsubscribe()
			connSubscription.Unsubscribe()
			return
		}
	}
}

func (m *Messenger) getPinnedMailserver() (*mailservers.Mailserver, error) {
	fleet, err := m.getFleet()
	if err != nil {
		return nil, err
	}

	pinnedMailservers, err := m.settings.GetPinnedMailservers()
	if err != nil {
		return nil, err
	}

	pinnedMailserver, ok := pinnedMailservers[fleet]
	if !ok {
		return nil, nil
	}

	customMailservers, err := m.mailservers.Mailservers()
	if err != nil {
		return nil, err
	}

	fleetMailservers := mailservers.DefaultMailservers()

	for _, c := range fleetMailservers {
		if c.Fleet == fleet && c.ID == pinnedMailserver {
			return &c, nil
		}
	}

	for _, c := range customMailservers {
		if c.Fleet == fleet && c.ID == pinnedMailserver {
			return &c, nil
		}
	}

	return nil, nil
}

func (m *Messenger) checkMailserverConnection() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.quit:
			return
		case <-ticker.C:
			m.mailserverCycle.Lock()

			canUseMailservers, err := m.settings.CanUseMailservers()
			if err != nil {
				m.mailserverCycle.Unlock()
				return
			}
			if !canUseMailservers {
				m.mailserverCycle.Unlock()
				continue
			}

			m.logger.Info("Verifying mailserver connection state...")

			pinnedMailserver, err := m.getPinnedMailserver()
			if err != nil {
				m.logger.Error("Could not obtain the pinned mailserver", zap.Error(err))
				m.mailserverCycle.Unlock()
				continue
			}

			if pinnedMailserver != nil {

				m.logger.Info("pinned mailserver not nil")
				activeMailserver := m.getActiveMailserver()
				if activeMailserver == nil || activeMailserver.ID != pinnedMailserver.ID {
					m.logger.Info("Connecting to mailserver")
					err = m.connectToMailserver(*pinnedMailserver)
					if err != nil {
						m.logger.Error("Could not connect to pinned mailserver", zap.Error(err))
						m.mailserverCycle.Unlock()
						continue
					}
				}
			} else {
				// or setup a random mailserver:
				if !m.isActiveMailserverAvailable() {
					m.cycleMailservers()
					m.logger.Info("No available active mailserver")
				}
			}
			m.mailserverCycle.Unlock()

		}
	}
}
