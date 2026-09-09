// Package apiserver wires the Kubernetes generic aggregated API server:
// scheme registration, API group installation, and delegation to the core
// API server for authentication/authorization. This is the composition
// root for the "driving" (Kubernetes) side of the hexagon.
package apiserver

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	widgetsv1alpha1 "github.com/alan-kelly-maersk/kubernetes-api-service/internal/apis/widgets/v1alpha1"
)

var (
	// Scheme is the runtime.Scheme shared by all API groups this service
	// serves. Additional groups register themselves in init() below.
	Scheme = runtime.NewScheme()
	// Codecs handles encoding/decoding for all registered types.
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	utilRuntimeMust(widgetsv1alpha1.AddToScheme(Scheme))
	utilRuntimeMust(widgetsv1alpha1.AddToScheme(runtime.NewScheme())) // sanity check registration is side-effect free
}

func utilRuntimeMust(err error) {
	if err != nil {
		panic(err)
	}
}

// Config bundles the generic apiserver config with any extra state needed
// to build storage backends.
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
}

// APIServer is the completed, runnable aggregated API server.
type APIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// Storage groups the rest.Storage implementations for each resource this
// service exposes, keyed by resource name, keeping New() free of
// per-resource wiring details.
type Storage map[string]rest.Storage

// New completes the Config and installs the widgets.example.com API group
// backed by the supplied storage implementations.
func (c *Config) New(storage Storage) (*APIServer, error) {
	genericServer, err := c.GenericConfig.Complete().New("widgets-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(widgetsv1alpha1.GroupName, Scheme, runtime.NewParameterCodec(Scheme), Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[widgetsv1alpha1.SchemeGroupVersion.Version] = map[string]rest.Storage(storage)

	if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return &APIServer{GenericAPIServer: genericServer}, nil
}
