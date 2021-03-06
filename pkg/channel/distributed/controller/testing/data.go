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

package testing

import (
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgotesting "k8s.io/client-go/testing"
	kafkav1beta1 "knative.dev/eventing-kafka/pkg/apis/messaging/v1beta1"
	"knative.dev/eventing-kafka/pkg/channel/distributed/common/config"
	commonconstants "knative.dev/eventing-kafka/pkg/channel/distributed/common/constants"
	commonenv "knative.dev/eventing-kafka/pkg/channel/distributed/common/env"
	"knative.dev/eventing-kafka/pkg/channel/distributed/common/health"
	kafkaconstants "knative.dev/eventing-kafka/pkg/channel/distributed/common/kafka/constants"
	kafkautil "knative.dev/eventing-kafka/pkg/channel/distributed/common/kafka/util"
	"knative.dev/eventing-kafka/pkg/channel/distributed/controller/constants"
	"knative.dev/eventing-kafka/pkg/channel/distributed/controller/env"
	"knative.dev/eventing-kafka/pkg/channel/distributed/controller/event"
	"knative.dev/eventing-kafka/pkg/channel/distributed/controller/util"
	"knative.dev/eventing/pkg/apis/messaging"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/logging"
	reconcilertesting "knative.dev/pkg/reconciler/testing"
	"knative.dev/pkg/system"
)

// Constants
const (
	// Prometheus MetricsPort
	MetricsPortName = "metrics"

	// Environment Test Data
	ServiceAccount           = "TestServiceAccount"
	KafkaAdminType           = "kafka"
	MetricsPort              = 9876
	MetricsDomain            = "eventing-kafka"
	HealthPort               = 8082
	ReceiverImage            = "TestReceiverImage"
	ReceiverReplicas         = 1
	DispatcherImage          = "TestDispatcherImage"
	DispatcherReplicas       = 1
	DefaultNumPartitions     = 4
	DefaultReplicationFactor = 1
	DefaultRetentionMillis   = 99999

	// Channel Test Data
	KafkaChannelNamespace  = "kafkachannel-namespace"
	KafkaChannelName       = "kafkachannel-name"
	KafkaChannelKey        = KafkaChannelNamespace + "/" + KafkaChannelName
	KafkaSecretNamespace   = commonconstants.KnativeEventingNamespace // Needs To Match Hardcoded Value In Reconciliation
	KafkaSecretName        = "kafkasecret-name"
	KafkaSecretKey         = KafkaSecretNamespace + "/" + KafkaSecretName
	ReceiverDeploymentName = KafkaSecretName + "-b9176d5f-receiver" // Truncated MD5 Hash Of KafkaSecretName
	ReceiverServiceName    = ReceiverDeploymentName
	TopicName              = KafkaChannelNamespace + "." + KafkaChannelName

	KafkaSecretDataValueBrokers  = "TestKafkaSecretDataBrokers"
	KafkaSecretDataValueUsername = "TestKafkaSecretDataUsername"
	KafkaSecretDataValuePassword = "TestKafkaSecretDataPassword"

	// ChannelSpec Test Data
	NumPartitions     = 123
	ReplicationFactor = 456

	// Test MetaData
	ErrorString   = "Expected Mock Test Error"
	SuccessString = "Expected Mock Test Success"

	// Test Dispatcher Resources
	DispatcherMemoryRequest = "20Mi"
	DispatcherCpuRequest    = "100m"
	DispatcherMemoryLimit   = "50Mi"
	DispatcherCpuLimit      = "300m"

	// Test Receiver Resources
	ReceiverMemoryRequest = "10Mi"
	ReceiverMemoryLimit   = "20Mi"
	ReceiverCpuRequest    = "10m"
	ReceiverCpuLimit      = "100m"

	ControllerConfigYaml = `
receiver:
  cpuLimit: 200m
  cpuRequest: 100m
  memoryLimit: 100Mi
  memoryRequest: 50Mi
  replicas: 1
dispatcher:
  cpuLimit: 500m
  cpuRequest: 300m
  memoryLimit: 128Mi
  memoryRequest: 50Mi
  replicas: 1
  retryInitialIntervalMillis: 500
  retryTimeMillis: 300000
  retryExponentialBackoff: true
kafka:
  topic:
    defaultNumPartitions: 4
    defaultReplicationFactor: 1
    defaultRetentionMillis: 604800000
  adminType: kafka
`
	SaramaConfigYaml = `
Version: 2.0.0
Admin:
  Timeout: 10000000000  # 10 seconds
Net:
  KeepAlive: 30000000000  # 30 seconds
  MaxOpenRequests: 1 # Set to 1 for use with Idempotent Producer
  TLS:
    Enable: false
  SASL:
    Enable: false
    Mechanism: PLAIN
    Version: 1
Metadata:
  RefreshFrequency: 300000000000
Consumer:
  Offsets:
    AutoCommit:
        Interval: 5000000000
    Retention: 604800000000000
Producer:
  Idempotent: true
  RequiredAcks: -1
`
)

var (
	DefaultRetentionMillisString = strconv.FormatInt(DefaultRetentionMillis, 10)
)

//
// ControllerConfig Test Data
//

// Set The Required Environment Variables
func NewEnvironment() *env.Environment {
	return &env.Environment{
		ServiceAccount:  ServiceAccount,
		MetricsPort:     MetricsPort,
		MetricsDomain:   MetricsDomain,
		DispatcherImage: DispatcherImage,
		ReceiverImage:   ReceiverImage,
	}
}

// Set The Required Config Fields
func NewConfig() *config.EventingKafkaConfig {
	return &config.EventingKafkaConfig{
		Dispatcher: config.EKDispatcherConfig{
			EKKubernetesConfig: config.EKKubernetesConfig{
				Replicas:      DispatcherReplicas,
				CpuLimit:      resource.MustParse(DispatcherCpuLimit),
				CpuRequest:    resource.MustParse(DispatcherCpuRequest),
				MemoryLimit:   resource.MustParse(DispatcherMemoryLimit),
				MemoryRequest: resource.MustParse(DispatcherMemoryRequest),
			},
		},
		Receiver: config.EKReceiverConfig{
			EKKubernetesConfig: config.EKKubernetesConfig{
				Replicas:      ReceiverReplicas,
				CpuLimit:      resource.MustParse(ReceiverCpuLimit),
				CpuRequest:    resource.MustParse(ReceiverCpuRequest),
				MemoryLimit:   resource.MustParse(ReceiverMemoryLimit),
				MemoryRequest: resource.MustParse(ReceiverMemoryRequest),
			},
		},
		Kafka: config.EKKafkaConfig{
			Topic: config.EKKafkaTopicConfig{
				DefaultNumPartitions:     DefaultNumPartitions,
				DefaultReplicationFactor: DefaultReplicationFactor,
				DefaultRetentionMillis:   DefaultRetentionMillis,
			},
			AdminType: KafkaAdminType,
		},
	}
}

//
// Kafka Secret Resources
//

// KafkaSecretOption Enables Customization Of A KafkaChannel
type KafkaSecretOption func(secret *corev1.Secret)

// Create A New Kafka Auth Secret For Testing
func NewKafkaSecret(options ...KafkaSecretOption) *corev1.Secret {

	// Create The Specified Kafka Secret
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       constants.SecretKind,
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      KafkaSecretName,
			Namespace: KafkaSecretNamespace,
		},
		Data: map[string][]byte{
			constants.KafkaSecretDataKeyBrokers:  []byte(KafkaSecretDataValueBrokers),
			constants.KafkaSecretDataKeyUsername: []byte(KafkaSecretDataValueUsername),
			constants.KafkaSecretDataKeyPassword: []byte(KafkaSecretDataValuePassword),
		},
		Type: "opaque",
	}

	// Apply The Specified Kafka secret Customizations
	for _, option := range options {
		option(secret)
	}

	// Return The Test Kafka Secret
	return secret

}

// Set The Kafka Secret's DeletionTimestamp To Current Time
func WithKafkaSecretDeleted(secret *corev1.Secret) {
	deleteTime := metav1.NewTime(time.Unix(1e9, 0))
	secret.ObjectMeta.SetDeletionTimestamp(&deleteTime)
}

// Set The Kafka Secret's Finalizer
func WithKafkaSecretFinalizer(secret *corev1.Secret) {
	secret.ObjectMeta.Finalizers = []string{constants.EventingKafkaFinalizerPrefix + "kafkasecrets.eventing-kafka.knative.dev"}
}

// Utility Function For Creating A PatchActionImpl For The Finalizer Patch Command
func NewKafkaSecretFinalizerPatchActionImpl() clientgotesting.PatchActionImpl {
	return clientgotesting.PatchActionImpl{
		ActionImpl: clientgotesting.ActionImpl{
			Namespace:   KafkaSecretNamespace,
			Verb:        "patch",
			Resource:    schema.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "secrets"},
			Subresource: "",
		},
		Name:      KafkaSecretName,
		PatchType: "application/merge-patch+json",
		Patch:     []byte(`{"metadata":{"finalizers":["eventing-kafka/kafkasecrets.eventing-kafka.knative.dev"],"resourceVersion":""}}`),
	}
}

// Utility Function For Creating A Successful Kafka Secret Reconciled Event
func NewKafkaSecretSuccessfulReconciliationEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, event.KafkaSecretReconciled.String(), fmt.Sprintf("Kafka Secret Reconciled Successfully: \"%s/%s\"", KafkaSecretNamespace, KafkaSecretName))
}

// Utility Function For Creating A Failed Kafka secret Reconciled Event
func NewKafkaSecretFailedReconciliationEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeWarning, "InternalError", constants.ReconciliationFailedError)
}

// Utility Function For Creating A Successful Kafka Secret Finalizer Update Event
func NewKafkaSecretSuccessfulFinalizedEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, event.KafkaSecretFinalized.String(), fmt.Sprintf("Kafka Secret Finalized Successfully: \"%s/%s\"", KafkaSecretNamespace, KafkaSecretName))
}

// Utility Function For Creating A Successful Kafka Secret Finalizer Update Event
func NewKafkaSecretFinalizerUpdateEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "%s" finalizers`, KafkaSecretName)
}

//
// KafkaChannel Resources
//

// KafkaChannelOption Enables Customization Of A KafkaChannel
type KafkaChannelOption func(*kafkav1beta1.KafkaChannel)

// Utility Function For Creating A Custom KafkaChannel For Testing
func NewKafkaChannel(options ...KafkaChannelOption) *kafkav1beta1.KafkaChannel {

	// Create The Specified KafkaChannel
	kafkachannel := &kafkav1beta1.KafkaChannel{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kafkav1beta1.SchemeGroupVersion.String(),
			Kind:       constants.KafkaChannelKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: KafkaChannelNamespace,
			Name:      KafkaChannelName,
		},
		Spec: kafkav1beta1.KafkaChannelSpec{
			NumPartitions:     NumPartitions,
			ReplicationFactor: ReplicationFactor,
			// TODO RetentionMillis:   RetentionMillis,
		},
	}

	// Apply The Specified KafkaChannel Customizations
	for _, option := range options {
		option(kafkachannel)
	}

	// Return The Test KafkaChannel
	return kafkachannel
}

// Set The KafkaChannel's Status To Initialized State
func WithInitializedConditions(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.InitializeConditions()
	kafkachannel.Status.MarkConfigTrue()
}

// Set The KafkaChannel's DeletionTimestamp To Current Time
func WithDeletionTimestamp(kafkachannel *kafkav1beta1.KafkaChannel) {
	deleteTime := metav1.NewTime(time.Unix(1e9, 0))
	kafkachannel.ObjectMeta.SetDeletionTimestamp(&deleteTime)
}

// Set The KafkaChannel's Finalizer
func WithFinalizer(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.ObjectMeta.Finalizers = []string{"kafkachannels.messaging.knative.dev"}
}

// Set The KafkaChannel's MetaData
func WithMetaData(kafkachannel *kafkav1beta1.KafkaChannel) {
	WithAnnotations(kafkachannel)
	WithLabels(kafkachannel)
}

// Set The KafkaChannel's Annotations
func WithAnnotations(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.ObjectMeta.Annotations = map[string]string{
		messaging.SubscribableDuckVersionAnnotation: constants.SubscribableDuckVersionAnnotationV1,
	}
}

// Set The KafkaChannel's Labels
func WithLabels(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.ObjectMeta.Labels = map[string]string{
		constants.KafkaTopicLabel:  fmt.Sprintf("%s.%s", KafkaChannelNamespace, KafkaChannelName),
		constants.KafkaSecretLabel: KafkaSecretName,
	}
}

// Set The KafkaChannel's Address
func WithAddress(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.SetAddress(&apis.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s-%s.%s.svc.cluster.local", KafkaChannelName, kafkaconstants.KafkaChannelServiceNameSuffix, KafkaChannelNamespace),
	})
}

// Set The KafkaChannel's Service As READY
func WithKafkaChannelServiceReady(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkChannelServiceTrue()
}

// Set The KafkaChannel's Services As Failed
func WithKafkaChannelServiceFailed(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkChannelServiceFailed(event.KafkaChannelServiceReconciliationFailed.String(), "Failed To Create KafkaChannel Service: inducing failure for create services")
}

// Set The KafkaChannel's Receiver Service As READY
func WithReceiverServiceReady(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkServiceTrue()
}

// Set The KafkaChannel's Receiver Service As Failed
func WithReceiverServiceFailed(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkServiceFailed(event.ReceiverServiceReconciliationFailed.String(), "Receiver Service Failed: inducing failure for create services")
}

// Set The KafkaChannel's Receiver Service As Finalized
func WithReceiverServiceFinalized(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkServiceFailed("ChannelServiceUnavailable", "Kafka Auth Secret Finalized")
}

// Set The KafkaChannel's Receiver Deployment As READY
func WithReceiverDeploymentReady(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkEndpointsTrue()
}

// Set The KafkaChannel's Receiver Deployment As Failed
func WithReceiverDeploymentFailed(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkEndpointsFailed(event.ReceiverDeploymentReconciliationFailed.String(), "Receiver Deployment Failed: inducing failure for create deployments")
}

// Set The KafkaChannel's Receiver Deployment As Finalized
func WithReceiverDeploymentFinalized(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkEndpointsFailed("ChannelDeploymentUnavailable", "Kafka Auth Secret Finalized")
}

// Set The KafkaChannel's Dispatcher Deployment As READY
func WithDispatcherDeploymentReady(kafkachannel *kafkav1beta1.KafkaChannel) {
	// TODO - This is unnecessary since the testing framework doesn't return any Status Conditions from the K8S commands (Create, Get)
	//        which means the propagate function doesn't do anything.  This is a testing gap with the framework and propagateDispatcherStatus()
	// kafkachannel.Status.PropagateDispatcherStatus()
}

// Set The KafkaChannel's Dispatcher Deployment As Failed
func WithDispatcherFailed(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkDispatcherFailed(event.DispatcherDeploymentReconciliationFailed.String(), "Failed To Create Dispatcher Deployment: inducing failure for create deployments")
}

// Set The KafkaChannel's Topic READY
func WithTopicReady(kafkachannel *kafkav1beta1.KafkaChannel) {
	kafkachannel.Status.MarkTopicTrue()
}

// Utility Function For Creating A Custom KafkaChannel "Channel" Service For Testing
func NewKafkaChannelService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       constants.ServiceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkautil.AppendKafkaChannelServiceNameSuffix(KafkaChannelName),
			Namespace: KafkaChannelNamespace,
			Labels: map[string]string{
				constants.KafkaChannelNameLabel:      KafkaChannelName,
				constants.KafkaChannelNamespaceLabel: KafkaChannelNamespace,
				constants.KafkaChannelReceiverLabel:  "true",
				constants.K8sAppChannelSelectorLabel: constants.K8sAppChannelSelectorValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				NewChannelOwnerRef(),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: ReceiverDeploymentName + "." + commonconstants.KnativeEventingNamespace + ".svc.cluster.local",
		},
	}
}

// Utility Function For Creating A Custom Receiver Service For Testing
func NewKafkaChannelReceiverService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       constants.ServiceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ReceiverDeploymentName,
			Namespace: commonconstants.KnativeEventingNamespace,
			Labels: map[string]string{
				"k8s-app":               "eventing-kafka-channels",
				"kafkachannel-receiver": "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				NewSecretOwnerRef(),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       constants.HttpPortName,
					Port:       constants.HttpServicePortNumber,
					TargetPort: intstr.FromInt(constants.HttpContainerPortNumber),
				},
				{
					Name:       MetricsPortName,
					Port:       MetricsPort,
					TargetPort: intstr.FromInt(MetricsPort),
				},
			},
			Selector: map[string]string{
				"app": ReceiverDeploymentName,
			},
		},
	}
}

// Utility Function For Creating A Receiver Deployment For The Test Channel
func NewKafkaChannelReceiverDeployment() *appsv1.Deployment {
	replicas := int32(ReceiverReplicas)
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       constants.DeploymentKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ReceiverDeploymentName,
			Namespace: commonconstants.KnativeEventingNamespace,
			Labels: map[string]string{
				"app":                   ReceiverDeploymentName,
				"kafkachannel-receiver": "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				NewSecretOwnerRef(),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": ReceiverDeploymentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": ReceiverDeploymentName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccount,
					Containers: []corev1.Container{
						{
							Name: ReceiverDeploymentName,
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.HealthPort),
										Path: health.LivenessPath,
									},
								},
								InitialDelaySeconds: constants.ChannelLivenessDelay,
								PeriodSeconds:       constants.ChannelLivenessPeriod,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.HealthPort),
										Path: health.ReadinessPath,
									},
								},
								InitialDelaySeconds: constants.ChannelReadinessDelay,
								PeriodSeconds:       constants.ChannelReadinessPeriod,
							},
							Image: ReceiverImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "server",
									ContainerPort: int32(8080),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  system.NamespaceEnvKey,
									Value: commonconstants.KnativeEventingNamespace,
								},
								{
									Name:  commonenv.KnativeLoggingConfigMapNameEnvVarKey,
									Value: logging.ConfigMapName(),
								},
								{
									Name:  commonenv.ServiceNameEnvVarKey,
									Value: ReceiverServiceName,
								},
								{
									Name:  commonenv.MetricsPortEnvVarKey,
									Value: strconv.Itoa(MetricsPort),
								},
								{
									Name:  commonenv.MetricsDomainEnvVarKey,
									Value: MetricsDomain,
								},
								{
									Name:  commonenv.HealthPortEnvVarKey,
									Value: strconv.Itoa(HealthPort),
								},
								{
									Name: commonenv.KafkaBrokerEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyBrokers,
										},
									},
								},
								{
									Name: commonenv.KafkaUsernameEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyUsername,
										},
									},
								},
								{
									Name: commonenv.KafkaPasswordEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyPassword,
										},
									},
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(ReceiverCpuRequest),
									corev1.ResourceMemory: resource.MustParse(ReceiverMemoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(ReceiverCpuLimit),
									corev1.ResourceMemory: resource.MustParse(ReceiverMemoryLimit),
								},
							},
						},
					},
				},
			},
		},
	}
}

// Utility Function For Creating A Custom KafkaChannel Dispatcher Service For Testing
func NewKafkaChannelDispatcherService() *corev1.Service {

	// Get The Expected Service Name For The Test KafkaChannel
	serviceName := util.DispatcherDnsSafeName(&kafkav1beta1.KafkaChannel{
		ObjectMeta: metav1.ObjectMeta{Namespace: KafkaChannelNamespace, Name: KafkaChannelName},
	})

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       constants.ServiceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: commonconstants.KnativeEventingNamespace,
			Labels: map[string]string{
				constants.KafkaChannelNameLabel:       KafkaChannelName,
				constants.KafkaChannelNamespaceLabel:  KafkaChannelNamespace,
				constants.KafkaChannelDispatcherLabel: "true",
				constants.K8sAppChannelSelectorLabel:  constants.K8sAppDispatcherSelectorValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				NewChannelOwnerRef(),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       MetricsPortName,
					Port:       int32(MetricsPort),
					TargetPort: intstr.FromInt(MetricsPort),
				},
			},
			Selector: map[string]string{
				"app": serviceName,
			},
		},
	}
}

// Utility Function For Creating A Custom KafkaChannel Dispatcher Deployment For Testing
func NewKafkaChannelDispatcherDeployment() *appsv1.Deployment {

	// Get The Expected Dispatcher & Topic Names For The Test KafkaChannel
	sparseKafkaChannel := &kafkav1beta1.KafkaChannel{ObjectMeta: metav1.ObjectMeta{Namespace: KafkaChannelNamespace, Name: KafkaChannelName}}
	dispatcherName := util.DispatcherDnsSafeName(sparseKafkaChannel)
	topicName := util.TopicName(sparseKafkaChannel)

	// Replicas Int Reference
	replicas := int32(DispatcherReplicas)

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       constants.DeploymentKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dispatcherName,
			Namespace: commonconstants.KnativeEventingNamespace,
			Labels: map[string]string{
				constants.AppLabel:                    dispatcherName,
				constants.KafkaChannelNameLabel:       KafkaChannelName,
				constants.KafkaChannelNamespaceLabel:  KafkaChannelNamespace,
				constants.KafkaChannelDispatcherLabel: "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				NewChannelOwnerRef(),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": dispatcherName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": dispatcherName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccount,
					Containers: []corev1.Container{
						{
							Name:  dispatcherName,
							Image: DispatcherImage,
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.HealthPort),
										Path: health.LivenessPath,
									},
								},
								InitialDelaySeconds: constants.DispatcherLivenessDelay,
								PeriodSeconds:       constants.DispatcherLivenessPeriod,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.HealthPort),
										Path: health.ReadinessPath,
									},
								},
								InitialDelaySeconds: constants.DispatcherReadinessDelay,
								PeriodSeconds:       constants.DispatcherReadinessPeriod,
							},
							Env: []corev1.EnvVar{
								{
									Name:  system.NamespaceEnvKey,
									Value: commonconstants.KnativeEventingNamespace,
								},
								{
									Name:  commonenv.KnativeLoggingConfigMapNameEnvVarKey,
									Value: logging.ConfigMapName(),
								},
								{
									Name:  commonenv.MetricsPortEnvVarKey,
									Value: strconv.Itoa(MetricsPort),
								},
								{
									Name:  commonenv.MetricsDomainEnvVarKey,
									Value: MetricsDomain,
								},
								{
									Name:  commonenv.HealthPortEnvVarKey,
									Value: strconv.Itoa(HealthPort),
								},
								{
									Name:  commonenv.ChannelKeyEnvVarKey,
									Value: fmt.Sprintf("%s/%s", KafkaChannelNamespace, KafkaChannelName),
								},
								{
									Name:  commonenv.ServiceNameEnvVarKey,
									Value: dispatcherName,
								},
								{
									Name:  commonenv.KafkaTopicEnvVarKey,
									Value: topicName,
								},
								{
									Name: commonenv.KafkaBrokerEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyBrokers,
										},
									},
								},
								{
									Name: commonenv.KafkaUsernameEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyUsername,
										},
									},
								},
								{
									Name: commonenv.KafkaPasswordEnvVarKey,
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: KafkaSecretName},
											Key:                  constants.KafkaSecretDataKeyPassword,
										},
									},
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse(DispatcherMemoryLimit),
									corev1.ResourceCPU:    resource.MustParse(DispatcherCpuLimit),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse(DispatcherMemoryRequest),
									corev1.ResourceCPU:    resource.MustParse(DispatcherCpuRequest),
								},
							},
						},
					},
				},
			},
		},
	}
}

// Utility Function For Creating A New OwnerReference Model For The Test Kafka Secret
func NewSecretOwnerRef() metav1.OwnerReference {
	blockOwnerDeletion := true
	controller := true
	return metav1.OwnerReference{
		APIVersion:         corev1.SchemeGroupVersion.String(),
		Kind:               constants.SecretKind,
		Name:               KafkaSecretName,
		UID:                "",
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &controller,
	}
}

// Utility Function For Creating A New OwnerReference Model For The Test Channel
func NewChannelOwnerRef() metav1.OwnerReference {
	blockOwnerDeletion := true
	controller := true
	return metav1.OwnerReference{
		APIVersion:         kafkav1beta1.SchemeGroupVersion.String(),
		Kind:               constants.KafkaChannelKind,
		Name:               KafkaChannelName,
		UID:                "",
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &controller,
	}
}

// Utility Function For Creating A UpdateActionImpl For The KafkaChannel Labels Update Command
func NewKafkaChannelLabelUpdate(kafkachannel *kafkav1beta1.KafkaChannel) clientgotesting.UpdateActionImpl {
	return clientgotesting.UpdateActionImpl{
		ActionImpl: clientgotesting.ActionImpl{
			Namespace:   "KafkaChannelNamespace",
			Verb:        "update",
			Resource:    schema.GroupVersionResource{Group: kafkav1beta1.SchemeGroupVersion.Group, Version: kafkav1beta1.SchemeGroupVersion.Version, Resource: "kafkachannels"},
			Subresource: "",
		},
		Object: kafkachannel,
	}
}

// Utility Function For Creating A PatchActionImpl For The Finalizer Patch Command
func NewFinalizerPatchActionImpl() clientgotesting.PatchActionImpl {
	return clientgotesting.PatchActionImpl{
		ActionImpl: clientgotesting.ActionImpl{
			Namespace:   KafkaChannelNamespace,
			Verb:        "patch",
			Resource:    schema.GroupVersionResource{Group: kafkav1beta1.SchemeGroupVersion.Group, Version: kafkav1beta1.SchemeGroupVersion.Version, Resource: "kafkachannels"},
			Subresource: "",
		},
		Name:      KafkaChannelName,
		PatchType: "application/merge-patch+json",
		Patch:     []byte(`{"metadata":{"finalizers":["kafkachannels.messaging.knative.dev"],"resourceVersion":""}}`),
		// Above finalizer name matches package private "defaultFinalizerName" constant in injection/reconciler/messaging/v1beta1/kafkachannel ;)
	}
}

// Utility Function For Creating A Successful KafkaChannel Reconciled Event
func NewKafkaChannelSuccessfulReconciliationEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, event.KafkaChannelReconciled.String(), `KafkaChannel Reconciled Successfully: "%s/%s"`, KafkaChannelNamespace, KafkaChannelName)
}

// Utility Function For Creating A Failed KafkaChannel Reconciled Event
func NewKafkaChannelFailedReconciliationEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeWarning, "InternalError", constants.ReconciliationFailedError)
}

// Utility Function For Creating A Successful KafkaChannel Finalizer Update Event
func NewKafkaChannelFinalizerUpdateEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", `Updated "%s" finalizers`, KafkaChannelName)
}

// Utility Function For Creating A Successful KafkaChannel Finalizer Update Event
func NewKafkaChannelSuccessfulFinalizedEvent() string {
	return reconcilertesting.Eventf(corev1.EventTypeNormal, event.KafkaChannelFinalized.String(), fmt.Sprintf("KafkaChannel Finalized Successfully: \"%s/%s\"", KafkaChannelNamespace, KafkaChannelName))
}
