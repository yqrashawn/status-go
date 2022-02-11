package protocol

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/status-im/go-waku/waku/v2/dnsdisc"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/params"
	"github.com/status-im/status-go/services/mailservers"
	"github.com/status-im/status-go/signal"
)

const defaultBackoff = 30 * time.Second

func (m *Messenger) mailserversByFleet(fleet string) []mailservers.Mailserver {
	var items []mailservers.Mailserver
	for _, ms := range mailserversMap() {
		m.logger.Info("fleet", zap.String("f1", fleet), zap.String("f2", ms.Fleet))
		if "eth."+ms.Fleet == fleet {
			items = append(items, ms)
		}
	}
	return items
}

func mailserversMap() []mailservers.Mailserver {

	m := []mailservers.Mailserver{
		mailservers.Mailserver{
			ID:      "mail-01.ac-cn-hongkong-c.eth.prod",
			Address: "enode://606ae04a71e5db868a722c77a21c8244ae38f1bd6e81687cc6cfe88a3063fa1c245692232f64f45bd5408fed5133eab8ed78049332b04f9c110eac7f71c1b429@47.75.247.214:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{

			ID:      "mail-01.do-ams3.eth.prod",
			Address: "enode://c42f368a23fa98ee546fd247220759062323249ef657d26d357a777443aec04db1b29a3a22ef3e7c548e18493ddaf51a31b0aed6079bd6ebe5ae838fcfaf3a49@178.128.142.54:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-01.gc-us-central1-a.eth.prod",
			Address: "enode://ee2b53b0ace9692167a410514bca3024695dbf0e1a68e1dff9716da620efb195f04a4b9e873fb9b74ac84de801106c465b8e2b6c4f0d93b8749d1578bfcaf03e@104.197.238.144:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-02.ac-cn-hongkong-c.eth.prod",
			Address: "enode://2c8de3cbb27a3d30cbb5b3e003bc722b126f5aef82e2052aaef032ca94e0c7ad219e533ba88c70585ebd802de206693255335b100307645ab5170e88620d2a81@47.244.221.14:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-02.do-ams3.eth.prod",
			Address: "enode://7aa648d6e855950b2e3d3bf220c496e0cae4adfddef3e1e6062e6b177aec93bc6cdcf1282cb40d1656932ebfdd565729da440368d7c4da7dbd4d004b1ac02bf8@178.128.142.26:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-02.gc-us-central1-a.eth.prod",
			Address: "enode://30211cbd81c25f07b03a0196d56e6ce4604bb13db773ff1c0ea2253547fafd6c06eae6ad3533e2ba39d59564cfbdbb5e2ce7c137a5ebb85e99dcfc7a75f99f55@23.236.58.92:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-03.ac-cn-hongkong-c.eth.prod",
			Address: "enode://e85f1d4209f2f99da801af18db8716e584a28ad0bdc47fbdcd8f26af74dbd97fc279144680553ec7cd9092afe683ddea1e0f9fc571ebcb4b1d857c03a088853d@47.244.129.82:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-03.do-ams3.eth.prod",
			Address: "enode://8a64b3c349a2e0ef4a32ea49609ed6eb3364be1110253c20adc17a3cebbc39a219e5d3e13b151c0eee5d8e0f9a8ba2cd026014e67b41a4ab7d1d5dd67ca27427@178.128.142.94:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-03.gc-us-central1-a.eth.prod",
			Address: "enode://44160e22e8b42bd32a06c1532165fa9e096eebedd7fa6d6e5f8bbef0440bc4a4591fe3651be68193a7ec029021cdb496cfe1d7f9f1dc69eb99226e6f39a7a5d4@35.225.221.245:443",
			Fleet:   "prod",
		},
		mailservers.Mailserver{
			ID:      "mail-01.ac-cn-hongkong-c.eth.staging",
			Address: "enode://b74859176c9751d314aeeffc26ec9f866a412752e7ddec91b19018a18e7cca8d637cfe2cedcb972f8eb64d816fbd5b4e89c7e8c7fd7df8a1329fa43db80b0bfe@47.52.90.156:443",
			Fleet:   "staging",
		},
		mailservers.Mailserver{
			ID:      "mail-01.do-ams3.eth.staging",
			Address: "enode://69f72baa7f1722d111a8c9c68c39a31430e9d567695f6108f31ccb6cd8f0adff4991e7fdca8fa770e75bc8a511a87d24690cbc80e008175f40c157d6f6788d48@206.189.240.16:443",
			Fleet:   "staging",
		},
		mailservers.Mailserver{
			ID:      "mail-01.gc-us-central1-a.eth.staging",
			Address: "enode://e4fc10c1f65c8aed83ac26bc1bfb21a45cc1a8550a58077c8d2de2a0e0cd18e40fd40f7e6f7d02dc6cd06982b014ce88d6e468725ffe2c138e958788d0002a7f@35.239.193.41:443",
			Fleet:   "staging",
		},
		mailservers.Mailserver{
			ID:      "mail-01.ac-cn-hongkong-c.eth.test",
			Address: "enode://619dbb5dda12e85bf0eb5db40fb3de625609043242737c0e975f7dfd659d85dc6d9a84f9461a728c5ab68c072fed38ca6a53917ca24b8e93cc27bdef3a1e79ac@47.52.188.196:443",
			Fleet:   "test",
		},
		mailservers.Mailserver{
			ID:      "mail-01.do-ams3.eth.test",
			Address: "enode://e4865fe6c2a9c1a563a6447990d8e9ce672644ae3e08277ce38ec1f1b690eef6320c07a5d60c3b629f5d4494f93d6b86a745a0bf64ab295bbf6579017adc6ed8@206.189.243.161:443",
			Fleet:   "test",
		},
		mailservers.Mailserver{
			ID:      "mail-01.gc-us-central1-a.eth.test",
			Address: "enode://707e57453acd3e488c44b9d0e17975371e2f8fb67525eae5baca9b9c8e06c86cde7c794a6c2e36203bf9f56cae8b0e50f3b33c4c2b694a7baeea1754464ce4e3@35.192.229.172:443",
			Fleet:   "test",
		},
	}
	return m
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

//TODO: error handling
func (m *Messenger) activeMailserverID() []byte {
	if m.mailserverCycle.activeMailserver == nil {
		return nil
	}

	id, err := m.mailserverCycle.activeMailserver.IDBytes()
	if err != nil {
		return nil
	}
	return id

}
func (m *Messenger) StartMailserverCycle() error {
	canUseMailservers, err := m.settings.CanUseMailservers()
	if err != nil {
		return err
	}
	if !canUseMailservers {
		return errors.New("mailserver use is not allowed")
	}

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

func (m *Messenger) disconnectV1Mailserver() error {
	// TODO: remove this function once WakuV1 is deprecated
	if m.mailserverCycle.activeMailserver == nil {
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

	node, err := m.mailserverCycle.activeMailserver.Enode()
	if err != nil {
		return err
	}
	m.server.RemovePeer(node)
	m.mailserverCycle.activeMailserver = nil
	return nil
}

func (m *Messenger) disconnectStoreNode() {
	if m.mailserverCycle.activeStoreNode == nil {
		return
	}
	m.logger.Info("Disconnecting active storeNode", zap.Any("nodeID", m.mailserverCycle.activeStoreNode.Pretty()))
	pInfo, ok := m.mailserverCycle.peers[string(*m.mailserverCycle.activeStoreNode)]
	if ok {
		pInfo.status = disconnected
		pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
		m.mailserverCycle.peers[string(*m.mailserverCycle.activeStoreNode)] = pInfo
	} else {
		m.mailserverCycle.peers[string(*m.mailserverCycle.activeStoreNode)] = peerStatus{
			status:          disconnected,
			canConnectAfter: time.Now().Add(defaultBackoff),
		}
	}

	err := m.transport.DropPeer(string(*m.mailserverCycle.activeStoreNode))
	if err != nil {
		m.logger.Warn("Could not drop peer")
	}

	m.mailserverCycle.activeStoreNode = nil
}

func (m *Messenger) disconnectActiveMailserver() {
	switch m.transport.WakuVersion() {
	case 1:
		m.disconnectV1Mailserver()
	case 2:
		m.disconnectStoreNode()
	}
	signal.SendMailserverChanged("", "")
}

func (m *Messenger) cycleMailservers() {
	m.mailserverCycle.Lock()
	defer m.mailserverCycle.Unlock()

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

func (m *Messenger) findNewMailserver() error {
	switch m.transport.WakuVersion() {
	case 1:
		return m.findNewMailserverV1()
	case 2:
		return m.findStoreNode()
	default:
		return errors.New("waku version is not supported")
	}
}

func (m *Messenger) findStoreNode() error {
	allMailservers := parseStoreNodeConfig(m.config.clusterConfig.StoreNodes)

	// TODO: append user mailservers once that functionality is available for waku2

	var mailserverList []multiaddr.Multiaddr
	now := time.Now()
	for _, node := range allMailservers {
		pID, err := getPeerID(node)
		if err != nil {
			continue
		}

		pInfo, ok := m.mailserverCycle.peers[string(pID)]
		if !ok || pInfo.canConnectAfter.Before(now) {
			mailserverList = append(mailserverList, node)
		}
	}

	m.logger.Info("Finding a new store node...")

	var mailserverStr []string
	for _, m := range mailserverList {
		mailserverStr = append(mailserverStr, m.String())
	}

	pingResult, err := mailservers.DoPing(context.Background(), mailserverStr, 500, mailservers.MultiAddressToAddress)
	if err != nil {
		return err
	}

	m.logger.Info("PING RESULT", zap.Any("ping", pingResult))

	var availableMailservers []*mailservers.PingResult
	for _, result := range pingResult {
		if result.Err != nil {
			continue // The results with error are ignored
		}
		availableMailservers = append(availableMailservers, result)
	}
	sort.Sort(byRTTMs(availableMailservers))

	if len(availableMailservers) == 0 {
		m.logger.Warn("No store nodes available, av =0 ") // Do nothing...
		return nil
	}

	m.logger.Info("AVAIBLE", zap.Int("av", len(availableMailservers)))

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

	return m.connectToStoreNode(parseMultiaddresses([]string{availableMailservers[r.Int64()].Address})[0])
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

func (m *Messenger) waitUntiMailserverAvailable() error {
	allMailservers, err := m.allMailserversV1()
	if err != nil {
		return err
	}
	var canConnectAfters []time.Time
	now := time.Now()
	for _, node := range allMailservers {
		pInfo, ok := m.mailserverCycle.peers[node.ID]
		// No info about mailserver, mailserver is available
		if !ok {
			m.logger.Info("Mailserver without info found, returning")
			return nil
		}
		canConnectAfters = append(canConnectAfters, pInfo.canConnectAfter)

	}
	sort.Slice(canConnectAfters, func(i, j int) bool {
		return canConnectAfters[i].Before(canConnectAfters[j])
	})

	m.logger.Info("Checking")
	// If the first is before, we can return
	if canConnectAfters[0].Before(now) {
		m.logger.Info("Before now")
		return nil
	}

	m.logger.Info("Waiting ")
	select {
	case <-time.After(canConnectAfters[0].Sub(now)):
		m.logger.Info("Waited")
		return nil
	}

	return nil

}

func (m *Messenger) allMailserversV1() ([]mailservers.Mailserver, error) {
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

func (m *Messenger) findNewMailserverV1() error {
	// TODO: remove this function once WakuV1 is deprecated

	// Append user mailservers
	fleet, err := m.getFleet()
	if err != nil {
		return err
	}

	allMailservers := m.mailserversByFleet(fleet)
	m.logger.Info("all mai", zap.Any("ha", allMailservers))

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

	m.logger.Info("PING RESULT", zap.Any("ping", mailserverStr))
	pingResult, err := mailservers.DoPing(context.Background(), mailserverStr, 500, mailservers.EnodeStringToAddr)
	if err != nil {
		return err
	}

	m.logger.Info("PING RESULT", zap.Any("ping", pingResult))
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
	var mailserverID string
	switch m.transport.WakuVersion() {
	case 1:
		if m.mailserverCycle.activeMailserver == nil {
			return disconnected, errors.New("Active mailserver is not set")
		}
		mailserverID = m.mailserverCycle.activeMailserver.ID
	case 2:
		if m.mailserverCycle.activeStoreNode == nil {
			return disconnected, errors.New("Active storenode is not set")
		}
		mailserverID = string(*m.mailserverCycle.activeStoreNode)
	default:
		return disconnected, errors.New("waku version is not supported")
	}

	return m.mailserverCycle.peers[mailserverID].status, nil
}

func (m *Messenger) connectToMailserver(ms mailservers.Mailserver) error {
	// TODO: remove this function once WakuV1 is deprecated

	if m.transport.WakuVersion() != 1 {
		return nil // This can only be used with wakuV1
	}

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
		node, err := ms.Enode()
		if err != nil {
			return err
		}
		m.server.AddPeer(node)
		if err := m.peerStore.Update([]*enode.Node{node}); err != nil {
			return err
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

	select {
	case <-ticker.C:
		activeMailserverStatus, err := m.activeMailserverStatus()
		if err != nil {
			return err
		}
		if activeMailserverStatus == connected {
			ticker.Stop()
			break
		}
	case <-time.After(60 * time.Second):
		ticker.Stop()
	}

	if nodeConnected {
		m.logger.Info("Mailserver available")
		signal.SendMailserverAvailable(m.mailserverCycle.activeMailserver.Address, m.mailserverCycle.activeMailserver.ID)
	}

	return nil
}

func (m *Messenger) connectToStoreNode(node multiaddr.Multiaddr) error {
	if m.transport.WakuVersion() != 2 {
		return nil // This can only be used with wakuV2
	}

	m.logger.Info("Connecting to storenode", zap.Any("peer", node))

	nodeConnected := false

	peerID, err := getPeerID(node)
	if err != nil {
		return err
	}

	m.mailserverCycle.activeStoreNode = &peerID
	signal.SendMailserverChanged(m.mailserverCycle.activeStoreNode.Pretty(), m.mailserverCycle.activeStoreNode.Pretty())

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
		if err := m.transport.DialPeer(node.String()); err != nil {
			return err
		}

		pInfo, ok := m.mailserverCycle.peers[string(peerID)]
		if ok {
			pInfo.status = connected
			pInfo.lastConnectionAttempt = time.Now()
		} else {
			m.mailserverCycle.peers[string(peerID)] = peerStatus{
				status:                connected,
				lastConnectionAttempt: time.Now(),
			}
		}

		nodeConnected = true
	}

	if nodeConnected {
		m.logger.Info("Storenode available")
		signal.SendMailserverAvailable(m.mailserverCycle.activeStoreNode.Pretty(), m.mailserverCycle.activeStoreNode.Pretty())
	}

	return nil
}

func (m *Messenger) getActiveMailserver() *mailservers.Mailserver {
	m.mailserverCycle.RLock()
	defer m.mailserverCycle.RUnlock()
	return m.mailserverCycle.activeMailserver
}

func (m *Messenger) isActiveMailserverAvailable() bool {
	m.mailserverCycle.RLock()
	defer m.mailserverCycle.RUnlock()

	mailserverStatus, err := m.activeMailserverStatus()
	if err != nil {
		return false
	}

	return mailserverStatus == connected
}

func (m *Messenger) updateWakuV2PeerStatus() {
	if m.transport.WakuVersion() != 2 {
		return // This can only be used with wakuV2
	}

	connSubscription, err := m.transport.SubscribeToConnStatusChanges()
	if err != nil {
		m.logger.Error("Could not subscribe to connection status changes", zap.Error(err))
	}

	for {
		select {
		case status := <-connSubscription.C:
			m.mailserverCycle.Lock()

			for pID, pInfo := range m.mailserverCycle.peers {
				if pInfo.status == disconnected {
					continue
				}

				// Removing disconnected

				found := false
				for connectedPeer := range status.Peers {
					peerID, err := peer.Decode(connectedPeer)
					if err != nil {
						continue
					}

					if string(peerID) == pID {
						found = true
						break
					}
				}
				if !found && pInfo.status == connected {
					m.logger.Info("Peer disconnected", zap.String("peer", peer.ID(pID).Pretty()))
					pInfo.status = disconnected
					pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
				}

				m.mailserverCycle.peers[pID] = pInfo
			}

			for connectedPeer := range status.Peers {
				peerID, err := peer.Decode(connectedPeer)
				if err != nil {
					continue
				}

				pInfo, ok := m.mailserverCycle.peers[string(peerID)]
				if !ok || pInfo.status != connected {
					m.logger.Info("Peer connected", zap.String("peer", connectedPeer))
					pInfo.status = connected
					pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
					m.mailserverCycle.peers[string(peerID)] = pInfo
				}
			}
			m.mailserverCycle.Unlock()

		case <-m.quit:
			connSubscription.Unsubscribe()
			return
		}
	}
}

func (m *Messenger) updateWakuV1PeerStatus() {
	// TODO: remove this function once WakuV1 is deprecated

	if m.transport.WakuVersion() != 1 {
		return // This can only be used with wakuV1
	}

	for {
		select {
		case <-m.mailserverCycle.events:
			connectedPeers := m.server.PeersInfo()
			m.mailserverCycle.Lock()

			for pID, pInfo := range m.mailserverCycle.peers {
				if pInfo.status == disconnected {
					continue
				}

				// Removing disconnected

				found := false
				for _, connectedPeer := range connectedPeers {
					idBytes, err := pInfo.mailserver.IDBytes()
					if err != nil {
						return
					}
					m.logger.Info("IDS", zap.Any("he", types.EncodeHex(idBytes)), zap.Any("be", connectedPeer.ID))
					if enode.HexID(connectedPeer.ID).String() == types.EncodeHex(idBytes)[2:] {
						found = true
						break
					}
				}
				if !found && (pInfo.status == connected || (pInfo.status == connecting && pInfo.lastConnectionAttempt.Add(8*time.Second).Before(time.Now()))) {
					m.logger.Info("Peer disconnected", zap.String("peer", enode.HexID(pID).String()))
					pInfo.status = disconnected
					pInfo.canConnectAfter = time.Now().Add(defaultBackoff)
				}

				m.mailserverCycle.peers[pID] = pInfo
			}

			for _, connectedPeer := range connectedPeers {
				hexID := enode.HexID(connectedPeer.ID).String()
				pInfo, ok := m.mailserverCycle.peers[hexID]
				if !ok || pInfo.status != connected {
					m.logger.Info("Peer connected", zap.String("peer", hexID))
					pInfo.status = connected
					pInfo.canConnectAfter = time.Now().Add(defaultBackoff)

					node, err := m.mailserverCycle.activeMailserver.Enode()
					if err != nil {
						m.logger.Error("failed to parse node", zap.Error(err))
						return
					}

					if m.mailserverCycle.activeMailserver != nil && hexID == node.ID().String() {
						m.logger.Info("Mailserver available")
						signal.SendMailserverAvailable(m.mailserverCycle.activeMailserver.Address, m.mailserverCycle.activeMailserver.ID)
					}
					m.mailserverCycle.peers[hexID] = pInfo
				}
			}
			m.mailserverCycle.Unlock()
		case <-m.quit:
			m.mailserverCycle.Lock()
			defer m.mailserverCycle.Unlock()
			close(m.mailserverCycle.events)
			m.mailserverCycle.subscription.Unsubscribe()
			return
		}
	}
}

func (m *Messenger) getPinnedMailserver() (*mailservers.Mailserver, error) {
	// TODO: Pinned mailservers are ony available in V1 for now
	if m.transport.WakuVersion() != 1 {
		return nil, nil
	}

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

	fleetMailservers := mailserversMap()

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
		m.logger.Info("Verifying mailserver connection state...")

		pinnedMailserver, err := m.getPinnedMailserver()
		if err != nil {
			m.logger.Error("Could not obtain the pinned mailserver", zap.Error(err))
			continue
		}

		if pinnedMailserver != nil {
			activeMailserver := m.getActiveMailserver()
			if activeMailserver == nil || activeMailserver.ID != pinnedMailserver.ID {
				m.logger.Info("New pinned mailserver", zap.Any("pinnedMailserver", pinnedMailserver))
				err = m.connectToMailserver(*pinnedMailserver)
				if err != nil {
					m.logger.Error("Could not connect to pinned mailserver", zap.Error(err))
					continue
				}
			}
		} else {
			// or setup a random mailserver:
			if !m.isActiveMailserverAvailable() {
				m.cycleMailservers()
			}
		}

		select {
		case <-m.quit:
			return
		case <-ticker.C:
			continue
		}
	}
}

func parseNodes(enodes []string) []*enode.Node {
	var nodes []*enode.Node
	for _, item := range enodes {
		parsedPeer, err := enode.ParseV4(item)
		if err == nil {
			nodes = append(nodes, parsedPeer)
		}
	}
	return nodes
}

func parseMultiaddresses(addresses []string) []multiaddr.Multiaddr {
	var result []multiaddr.Multiaddr
	for _, item := range addresses {
		ma, err := multiaddr.NewMultiaddr(item)
		if err == nil {
			result = append(result, ma)
		}
	}
	return result
}

func parseStoreNodeConfig(addresses []string) []multiaddr.Multiaddr {
	// TODO: once a scoring/reputation mechanism is added to waku,
	// this function can be modified to retrieve the storenodes
	// from waku peerstore.
	// We don't do that now because we can't trust any random storenode
	// So we use only those specified in the cluster config
	var result []multiaddr.Multiaddr
	var dnsDiscWg sync.WaitGroup

	maChan := make(chan multiaddr.Multiaddr, 1000)

	for _, addrString := range addresses {
		if strings.HasPrefix(addrString, "enrtree://") {
			// Use DNS Discovery
			dnsDiscWg.Add(1)
			go func(addr string) {
				defer dnsDiscWg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				multiaddresses, err := dnsdisc.RetrieveNodes(ctx, addr)
				if err == nil {
					for _, ma := range multiaddresses {
						maChan <- ma
					}
				}
			}(addrString)

		} else {
			// It's a normal multiaddress
			ma, err := multiaddr.NewMultiaddr(addrString)
			if err == nil {
				maChan <- ma
			}
		}
	}
	dnsDiscWg.Wait()
	close(maChan)
	for ma := range maChan {
		result = append(result, ma)
	}

	return result
}

func getPeerID(addr multiaddr.Multiaddr) (peer.ID, error) {
	idStr, err := addr.ValueForProtocol(multiaddr.P_P2P)
	if err != nil {
		return "", err
	}
	return peer.Decode(idStr)
}
