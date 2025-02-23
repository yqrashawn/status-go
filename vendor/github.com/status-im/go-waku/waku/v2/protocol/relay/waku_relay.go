package relay

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	proto "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/protocol"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.uber.org/zap"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	v2 "github.com/status-im/go-waku/waku/v2"
	"github.com/status-im/go-waku/waku/v2/metrics"
	waku_proto "github.com/status-im/go-waku/waku/v2/protocol"
	"github.com/status-im/go-waku/waku/v2/protocol/pb"
)

const WakuRelayID_v200 = protocol.ID("/vac/waku/relay/2.0.0")

var DefaultWakuTopic string = waku_proto.DefaultPubsubTopic().String()

type WakuRelay struct {
	host   host.Host
	pubsub *pubsub.PubSub

	log *zap.SugaredLogger

	bcaster v2.Broadcaster

	minPeersToPublish int

	// TODO: convert to concurrent maps
	topicsMutex     sync.Mutex
	wakuRelayTopics map[string]*pubsub.Topic
	relaySubs       map[string]*pubsub.Subscription

	// TODO: convert to concurrent maps
	subscriptions      map[string][]*Subscription
	subscriptionsMutex sync.Mutex
}

// Once https://github.com/status-im/nim-waku/issues/420 is fixed, implement a custom messageIdFn
func msgIdFn(pmsg *pubsub_pb.Message) string {
	hash := sha256.Sum256(pmsg.Data)
	return string(hash[:])
}

func NewWakuRelay(ctx context.Context, h host.Host, bcaster v2.Broadcaster, minPeersToPublish int, log *zap.SugaredLogger, opts ...pubsub.Option) (*WakuRelay, error) {
	w := new(WakuRelay)
	w.host = h
	w.wakuRelayTopics = make(map[string]*pubsub.Topic)
	w.relaySubs = make(map[string]*pubsub.Subscription)
	w.subscriptions = make(map[string][]*Subscription)
	w.bcaster = bcaster
	w.minPeersToPublish = minPeersToPublish
	w.log = log.Named("relay")

	// default options required by WakuRelay
	opts = append(opts, pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign))
	opts = append(opts, pubsub.WithNoAuthor())
	opts = append(opts, pubsub.WithMessageIdFn(msgIdFn))

	opts = append(opts, pubsub.WithGossipSubProtocols(
		[]protocol.ID{pubsub.GossipSubID_v11, pubsub.GossipSubID_v10, pubsub.FloodSubID, WakuRelayID_v200},
		func(feat pubsub.GossipSubFeature, proto protocol.ID) bool {
			switch feat {
			case pubsub.GossipSubFeatureMesh:
				return proto == pubsub.GossipSubID_v11 || proto == pubsub.GossipSubID_v10
			case pubsub.GossipSubFeaturePX:
				return proto == pubsub.GossipSubID_v11
			default:
				return false
			}
		},
	))

	ps, err := pubsub.NewGossipSub(ctx, h, opts...)
	if err != nil {
		return nil, err
	}
	w.pubsub = ps

	w.log.Info("Relay protocol started")

	return w, nil
}

func (w *WakuRelay) PubSub() *pubsub.PubSub {
	return w.pubsub
}

func (w *WakuRelay) Topics() []string {
	defer w.topicsMutex.Unlock()
	w.topicsMutex.Lock()

	var result []string
	for topic := range w.relaySubs {
		result = append(result, topic)
	}
	return result
}

func (w *WakuRelay) SetPubSub(pubSub *pubsub.PubSub) {
	w.pubsub = pubSub
}

func (w *WakuRelay) upsertTopic(topic string) (*pubsub.Topic, error) {
	defer w.topicsMutex.Unlock()
	w.topicsMutex.Lock()

	pubSubTopic, ok := w.wakuRelayTopics[topic]
	if !ok { // Joins topic if node hasn't joined yet
		newTopic, err := w.pubsub.Join(string(topic))
		if err != nil {
			return nil, err
		}
		w.wakuRelayTopics[topic] = newTopic
		pubSubTopic = newTopic
	}
	return pubSubTopic, nil
}

func (w *WakuRelay) subscribe(topic string) (subs *pubsub.Subscription, err error) {
	sub, ok := w.relaySubs[topic]
	if !ok {
		pubSubTopic, err := w.upsertTopic(topic)
		if err != nil {
			return nil, err
		}

		sub, err = pubSubTopic.Subscribe()
		if err != nil {
			return nil, err
		}
		w.relaySubs[topic] = sub

		w.log.Info("Subscribing to topic ", topic)
	}

	return sub, nil
}

func (w *WakuRelay) PublishToTopic(ctx context.Context, message *pb.WakuMessage, topic string) ([]byte, error) {
	// Publish a `WakuMessage` to a PubSub topic.
	if w.pubsub == nil {
		return nil, errors.New("PubSub hasn't been set")
	}

	if message == nil {
		return nil, errors.New("message can't be null")
	}

	if !w.EnoughPeersToPublishToTopic(topic) {
		return nil, errors.New("not enougth peers to publish")
	}

	pubSubTopic, err := w.upsertTopic(topic)

	if err != nil {
		return nil, err
	}

	out, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}

	err = pubSubTopic.Publish(ctx, out)
	if err != nil {
		return nil, err
	}

	hash := pb.Hash(out)

	return hash, nil
}

func (w *WakuRelay) Publish(ctx context.Context, message *pb.WakuMessage) ([]byte, error) {
	return w.PublishToTopic(ctx, message, DefaultWakuTopic)
}

func (w *WakuRelay) Stop() {
	w.host.RemoveStreamHandler(WakuRelayID_v200)
	w.subscriptionsMutex.Lock()
	defer w.subscriptionsMutex.Unlock()

	for _, topic := range w.Topics() {
		for _, sub := range w.subscriptions[topic] {
			sub.Unsubscribe()
		}
	}
	w.subscriptions = nil
}

func (w *WakuRelay) EnoughPeersToPublish() bool {
	return w.EnoughPeersToPublishToTopic(DefaultWakuTopic)
}

func (w *WakuRelay) EnoughPeersToPublishToTopic(topic string) bool {
	return len(w.PubSub().ListPeers(topic)) >= w.minPeersToPublish
}

func (w *WakuRelay) SubscribeToTopic(ctx context.Context, topic string) (*Subscription, error) {
	// Subscribes to a PubSub topic.
	// NOTE The data field SHOULD be decoded as a WakuMessage.
	sub, err := w.subscribe(topic)

	if err != nil {
		return nil, err
	}

	// Create client subscription
	subscription := new(Subscription)
	subscription.closed = false
	subscription.C = make(chan *waku_proto.Envelope, 1024) // To avoid blocking
	subscription.quit = make(chan struct{})

	w.subscriptionsMutex.Lock()
	defer w.subscriptionsMutex.Unlock()

	w.subscriptions[topic] = append(w.subscriptions[topic], subscription)

	if w.bcaster != nil {
		w.bcaster.Register(subscription.C)
	}

	go w.subscribeToTopic(topic, subscription, sub)

	return subscription, nil
}

func (w *WakuRelay) Subscribe(ctx context.Context) (*Subscription, error) {
	return w.SubscribeToTopic(ctx, DefaultWakuTopic)
}

func (w *WakuRelay) Unsubscribe(ctx context.Context, topic string) error {
	if _, ok := w.relaySubs[topic]; !ok {
		return fmt.Errorf("topics %s is not subscribed", (string)(topic))
	}
	w.log.Info("Unsubscribing from topic ", topic)

	for _, sub := range w.subscriptions[topic] {
		sub.Unsubscribe()
	}

	w.relaySubs[topic].Cancel()
	delete(w.relaySubs, topic)

	err := w.wakuRelayTopics[topic].Close()
	if err != nil {
		return err
	}
	delete(w.wakuRelayTopics, topic)

	return nil
}

func (w *WakuRelay) nextMessage(ctx context.Context, sub *pubsub.Subscription) <-chan *pubsub.Message {
	msgChannel := make(chan *pubsub.Message, 1024)
	go func(msgChannel chan *pubsub.Message) {
		defer func() {
			if r := recover(); r != nil {
				w.log.Debug("recovered msgChannel")
			}
		}()

		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				w.log.Error(fmt.Errorf("subscription failed: %w", err))
				sub.Cancel()
				close(msgChannel)
				for _, subscription := range w.subscriptions[sub.Topic()] {
					subscription.Unsubscribe()
				}
			}

			msgChannel <- msg
		}
	}(msgChannel)
	return msgChannel
}

func (w *WakuRelay) subscribeToTopic(t string, subscription *Subscription, sub *pubsub.Subscription) {
	ctx, err := tag.New(context.Background(), tag.Insert(metrics.KeyType, "relay"))
	if err != nil {
		w.log.Error(err)
		return
	}

	subChannel := w.nextMessage(ctx, sub)

	for {
		select {
		case <-subscription.quit:
			func() {
				subscription.Lock()
				defer subscription.Unlock()

				if subscription.closed {
					return
				}
				subscription.closed = true
				if w.bcaster != nil {
					<-w.bcaster.WaitUnregister(subscription.C) // Remove from broadcast list
				}

				close(subscription.C)
			}()
			// TODO: if there are no more relay subscriptions, close the pubsub subscription
		case msg := <-subChannel:
			if msg == nil {
				return
			}
			stats.Record(ctx, metrics.Messages.M(1))
			wakuMessage := &pb.WakuMessage{}
			if err := proto.Unmarshal(msg.Data, wakuMessage); err != nil {
				w.log.Error("could not decode message", err)
				return
			}

			envelope := waku_proto.NewEnvelope(wakuMessage, string(t))

			if w.bcaster != nil {
				w.bcaster.Submit(envelope)
			}
		}
	}
}
