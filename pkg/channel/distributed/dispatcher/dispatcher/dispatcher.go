/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dispatcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/Shopify/sarama"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/eventing-kafka/pkg/channel/distributed/common/kafka/consumer"
	kafkasarama "knative.dev/eventing-kafka/pkg/channel/distributed/common/kafka/sarama"
	"knative.dev/eventing-kafka/pkg/channel/distributed/common/metrics"
	eventingduck "knative.dev/eventing/pkg/apis/duck/v1"
	"knative.dev/eventing/pkg/channel"
)

// Define A Dispatcher Config Struct To Hold Configuration
type DispatcherConfig struct {
	Logger          *zap.Logger
	ClientId        string
	Brokers         []string
	Topic           string
	Username        string
	Password        string
	ChannelKey      string
	StatsReporter   metrics.StatsReporter
	SaramaConfig    *sarama.Config
	SubscriberSpecs []eventingduck.SubscriberSpec
}

// Knative Eventing SubscriberSpec Wrapper Enhanced With Sarama ConsumerGroup
type SubscriberWrapper struct {
	eventingduck.SubscriberSpec
	GroupId       string
	ConsumerGroup sarama.ConsumerGroup
	StopChan      chan struct{}
}

// SubscriberWrapper Constructor
func NewSubscriberWrapper(subscriberSpec eventingduck.SubscriberSpec, groupId string, consumerGroup sarama.ConsumerGroup) *SubscriberWrapper {
	return &SubscriberWrapper{subscriberSpec, groupId, consumerGroup, make(chan struct{})}
}

//  Dispatcher Interface
type Dispatcher interface {
	ConfigChanged(*v1.ConfigMap) Dispatcher
	Shutdown()
	UpdateSubscriptions(subscriberSpecs []eventingduck.SubscriberSpec) map[eventingduck.SubscriberSpec]error
}

// Define A DispatcherImpl Struct With Configuration & ConsumerGroup State
type DispatcherImpl struct {
	DispatcherConfig
	subscribers        map[types.UID]*SubscriberWrapper
	consumerUpdateLock sync.Mutex
	messageDispatcher  channel.MessageDispatcher
}

// Verify The DispatcherImpl Implements The Dispatcher Interface
var _ Dispatcher = &DispatcherImpl{}

// Dispatcher Constructor
func NewDispatcher(dispatcherConfig DispatcherConfig) Dispatcher {

	// Create The DispatcherImpl With Specified Configuration
	dispatcher := &DispatcherImpl{
		DispatcherConfig:  dispatcherConfig,
		subscribers:       make(map[types.UID]*SubscriberWrapper),
		messageDispatcher: channel.NewMessageDispatcher(dispatcherConfig.Logger),
	}

	// Return The DispatcherImpl
	return dispatcher
}

// Shutdown The Dispatcher
func (d *DispatcherImpl) Shutdown() {

	// Close ConsumerGroups Of All Subscriptions
	for _, subscriber := range d.subscribers {
		d.closeConsumerGroup(subscriber)
	}
}

// Update The Dispatcher's Subscriptions To Align With New State
func (d *DispatcherImpl) UpdateSubscriptions(subscriberSpecs []eventingduck.SubscriberSpec) map[eventingduck.SubscriberSpec]error {

	if d.SaramaConfig == nil {
		d.Logger.Error("Dispatcher has no config!")
		return nil
	}

	// Maps For Tracking Subscriber State
	activeSubscriptions := make(map[types.UID]bool)
	failedSubscriptions := make(map[eventingduck.SubscriberSpec]error)

	// Thread Safe ;)
	d.consumerUpdateLock.Lock()
	defer d.consumerUpdateLock.Unlock()

	// Loop Over All All The Specified Subscribers
	for _, subscriberSpec := range subscriberSpecs {

		// If The Subscriber Wrapper For The SubscriberSpec Does Not Exist Then Create One
		if _, ok := d.subscribers[subscriberSpec.UID]; !ok {

			// Format The GroupId For The Specified Subscriber
			groupId := fmt.Sprintf("kafka.%s", subscriberSpec.UID)

			// Create A ConsumerGroup Logger
			logger := d.Logger.With(zap.String("GroupId", groupId))

			// Attempt To Create A Kafka ConsumerGroup
			consumerGroup, _, err := consumer.CreateConsumerGroup(d.Brokers, d.SaramaConfig, groupId)
			if err != nil {

				// Log & Return Failure
				logger.Error("Failed To Create ConsumerGroup", zap.Error(err))
				failedSubscriptions[subscriberSpec] = err

			} else {

				// Create A New SubscriberWrapper With The ConsumerGroup
				subscriber := NewSubscriberWrapper(subscriberSpec, groupId, consumerGroup)

				// Should start observing metrics from Sarama Config.MetricsRegistry from CreateConsumerGroup() above ; )

				// Start The ConsumerGroup Processing Messages
				d.startConsuming(subscriber)

				// Track The New SubscriberWrapper For The SubscriberSpec As Active
				d.subscribers[subscriberSpec.UID] = subscriber
				activeSubscriptions[subscriberSpec.UID] = true
			}

		} else {

			// Otherwise Just Add To List Of Active Subscribers
			activeSubscriptions[subscriberSpec.UID] = true
		}
	}

	// Save the current (active) subscriber specs so that ConfigChanged() can use them to recreate the Dispatcher
	// if necessary without going through the inactive subscribers again.
	d.SubscriberSpecs = []eventingduck.SubscriberSpec{}

	// Close ConsumerGroups For Removed Subscriptions (In Map But No Longer Active)
	for _, subscriber := range d.subscribers {
		if !activeSubscriptions[subscriber.UID] {
			d.closeConsumerGroup(subscriber)
		} else {
			d.SubscriberSpecs = append(d.SubscriberSpecs, subscriber.SubscriberSpec)
		}
	}

	// Return Any Failed Subscriber Errors
	return failedSubscriptions
}

// Start Consuming Messages With The Specified Subscriber's ConsumerGroup
func (d *DispatcherImpl) startConsuming(subscriber *SubscriberWrapper) {

	// Validate The Subscriber / ConsumerGroup
	if subscriber != nil && subscriber.ConsumerGroup != nil {

		// Setup The ConsumerGroup Level Logger
		logger := d.Logger.With(zap.String("GroupId", subscriber.GroupId))

		// Asynchronously Process ConsumerGroup's Error Channel
		go func() {
			logger.Info("ConsumerGroup Error Processing Initiated")
			for err := range subscriber.ConsumerGroup.Errors() { // Closing ConsumerGroup Will Break Out Of This
				logger.Error("ConsumerGroup Error", zap.Error(err))
			}
			logger.Info("ConsumerGroup Error Processing Terminated")
		}()

		// Create A New ConsumerGroupHandler To Consume Messages With
		handler := NewHandler(logger, &subscriber.SubscriberSpec)

		// Consume Messages Asynchronously
		go func() {

			// Infinite Loop To Support Server-Side ConsumerGroup Re-Balance Which Ends Consume() Execution
			ctx := context.Background()
			for {
				select {

				// Non-Blocking Stop Channel Check
				case <-subscriber.StopChan:
					logger.Info("ConsumerGroup Closed - Ceasing Consumption")
					return

				// Start ConsumerGroup Consumption
				default:
					logger.Info("ConsumerGroup Message Consumption Initiated")
					err := subscriber.ConsumerGroup.Consume(ctx, []string{d.Topic}, handler)
					if err != nil {
						if err == sarama.ErrClosedConsumerGroup {
							logger.Info("ConsumerGroup Closed Error - Ceasing Consumption") // Should be caught above but here as added precaution.
							break
						} else {
							logger.Error("ConsumerGroup Failed To Consume Messages", zap.Error(err))
						}
					}
				}
			}
		}()
	}
}

// Close The ConsumerGroup Associated With A Single Subscriber
func (d *DispatcherImpl) closeConsumerGroup(subscriber *SubscriberWrapper) {

	// Get The ConsumerGroup Associated with The Specified Subscriber
	consumerGroup := subscriber.ConsumerGroup

	// Create Logger With GroupId & Subscriber URI
	logger := d.Logger.With(zap.String("GroupId", subscriber.GroupId), zap.String("URI", subscriber.SubscriberURI.String()))

	// If The ConsumerGroup Is Valid
	if consumerGroup != nil {

		// Mark The Subscriber's ConsumerGroup As Stopped
		close(subscriber.StopChan)

		// Close The ConsumerGroup
		err := consumerGroup.Close()
		if err != nil {
			// Simply Log ConsumerGroup Close Failures
			//   - Don't include in failedSubscriptions response as that is used to update Subscription Status.
			//   - Don't delete from ConsumerGroups Map to force retry of Close next time around.
			logger.Error("Failed To Close ConsumerGroup", zap.Error(err))
		} else {
			logger.Info("Successfully Closed ConsumerGroup")
			delete(d.subscribers, subscriber.UID)
		}
	} else {
		logger.Warn("Successfully Closed Subscriber With Nil ConsumerGroup")
		delete(d.subscribers, subscriber.UID)
	}
}

// ConfigChanged is called by the configMapObserver handler function in main() so that
// settings specific to the dispatcher may be extracted and the ConsumerGroups restarted if necessary.
// The new configmap could technically have changes to the eventing-kafka section as well as the sarama
// section, but none of those matter to a currently-running Dispatcher, so those are ignored here
// (which avoids the necessity of calling env.GetEnvironment and env.VerifyOverrides).  If those settings
// are needed in the future, the environment will also need to be re-parsed here.
// If there aren't any consumer-specific differences between the current config and the new one,
// then just log that and move on; do not restart the ConsumerGroups unnecessarily.
func (d *DispatcherImpl) ConfigChanged(configMap *v1.ConfigMap) Dispatcher {

	// Create A New Sarama Config
	d.Logger.Debug("New ConfigMap Received", zap.String("configMap.Name", configMap.ObjectMeta.Name))

	newConfig, err := kafkasarama.MergeSaramaSettings(nil, configMap)
	if err != nil {
		d.Logger.Error("Unable to merge sarama settings", zap.Error(err))
		return nil
	}

	// Validate Configuration (Should Always Be Present)
	if d.SaramaConfig != nil {

		// Some of the current config settings may not be overridden by the configmap (username, password, etc.)
		kafkasarama.UpdateSaramaConfig(newConfig, d.SaramaConfig.ClientID, d.SaramaConfig.Net.SASL.User, d.SaramaConfig.Net.SASL.Password)

		// Ignore the "Producer" section as changes to that do not require recreating the Dispatcher
		if kafkasarama.ConfigEqual(newConfig, d.SaramaConfig, newConfig.Producer) {
			d.Logger.Info("No Consumer Changes Detected In New Configuration - Ignoring")
			return nil
		}
	}

	// Create A New Dispatcher With The New Configuration (Reusing All Other Existing Config)
	d.Logger.Info("Consumer Changes Detected In New Configuration - Recreating Dispatcher")
	d.Shutdown()
	d.DispatcherConfig.SaramaConfig = newConfig
	newDispatcher := NewDispatcher(d.DispatcherConfig)
	failedSubscriptions := newDispatcher.UpdateSubscriptions(d.SubscriberSpecs)
	if len(failedSubscriptions) > 0 {
		d.Logger.Fatal("Failed To Subscribe Kafka Subscriptions For New Dispatcher", zap.Int("Count", len(failedSubscriptions)))
		return nil
	}
	return newDispatcher
}
