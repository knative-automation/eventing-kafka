module knative.dev/eventing-kafka

go 1.14

require (
	github.com/Azure/azure-event-hubs-go v1.3.1
	github.com/Shopify/sarama v1.27.0
	github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2 v2.2.0
	github.com/cloudevents/sdk-go/v2 v2.2.0
	github.com/davecgh/go-spew v1.1.1
	github.com/ghodss/yaml v1.0.0
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.2
	github.com/google/uuid v1.1.1
	github.com/influxdata/tdigest v0.0.1 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/slinkydeveloper/loadastic v0.0.0-20191203132749-9afe5a010a57
	github.com/stretchr/testify v1.6.0
	go.opencensus.io v0.22.5-0.20200716030834-3456e1d174b2
	go.uber.org/zap v1.15.0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.8 // indirect
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/utils v0.0.0-20200603063816-c1c6865ac451
	knative.dev/eventing v0.18.1-0.20201015095656-0a79d05b949a
	knative.dev/pkg v0.0.0-20201014184531-9e655c6b02f6
	knative.dev/test-infra v0.0.0-20201014021030-ae3984a33f82
	pack.ag/amqp v0.12.4 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
	k8s.io/code-generator => k8s.io/code-generator v0.18.8
)
