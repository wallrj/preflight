package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/jetstack/preflight/pkg/datagatherer"
	dgerror "github.com/jetstack/preflight/pkg/datagatherer/error"
	"github.com/pkg/errors"
	statusError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
)

// ConfigDynamic contains the configuration for the data-gatherer.
type ConfigDynamic struct {
	// KubeConfigPath is the path to the kubeconfig file. If empty, will assume it runs in-cluster.
	KubeConfigPath string `yaml:"kubeconfig"`
	// GroupVersionResource identifies the resource type to gather.
	GroupVersionResource schema.GroupVersionResource
	// ExcludeNamespaces is a list of namespaces to exclude.
	ExcludeNamespaces []string `yaml:"exclude-namespaces"`
	// IncludeNamespaces is a list of namespaces to include.
	IncludeNamespaces []string `yaml:"include-namespaces"`
}

// UnmarshalYAML unmarshals the ConfigDynamic resolving GroupVersionResource.
func (c *ConfigDynamic) UnmarshalYAML(unmarshal func(interface{}) error) error {
	aux := struct {
		KubeConfigPath string `yaml:"kubeconfig"`
		ResourceType   struct {
			Group    string `yaml:"group"`
			Version  string `yaml:"version"`
			Resource string `yaml:"resource"`
		} `yaml:"resource-type"`
		ExcludeNamespaces []string `yaml:"exclude-namespaces"`
		IncludeNamespaces []string `yaml:"include-namespaces"`
	}{}
	err := unmarshal(&aux)
	if err != nil {
		return err
	}

	c.KubeConfigPath = aux.KubeConfigPath
	c.GroupVersionResource.Group = aux.ResourceType.Group
	c.GroupVersionResource.Version = aux.ResourceType.Version
	c.GroupVersionResource.Resource = aux.ResourceType.Resource
	c.ExcludeNamespaces = aux.ExcludeNamespaces
	c.IncludeNamespaces = aux.IncludeNamespaces

	return nil
}

// validate validates the configuration.
func (c *ConfigDynamic) validate() error {
	var errors []string
	if len(c.ExcludeNamespaces) > 0 && len(c.IncludeNamespaces) > 0 {
		errors = append(errors, "cannot set excluded and included namespaces")
	}

	if c.GroupVersionResource.Resource == "" {
		errors = append(errors, "invalid configuration: GroupVersionResource.Resource cannot be empty")
	}

	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, ", "))
	}

	return nil
}

// NewDataGatherer constructs a new instance of the generic K8s data-gatherer for the provided
// GroupVersionResource.
func (c *ConfigDynamic) NewDataGatherer(ctx context.Context) (datagatherer.DataGatherer, error) {
	cl, err := NewDynamicClient(c.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	return c.newDataGathererWithClient(ctx, cl)
}

func (c *ConfigDynamic) newDataGathererWithClient(ctx context.Context, cl dynamic.Interface) (datagatherer.DataGatherer, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	return &DataGathererDynamic{
		ctx:                  ctx,
		cl:                   cl,
		groupVersionResource: c.GroupVersionResource,
		fieldSelector:        generateFieldSelector(c.ExcludeNamespaces),
		namespaces:           c.IncludeNamespaces,
	}, nil
}

// DataGathererDynamic is a generic gatherer for Kubernetes. It knows how to request
// a list of generic resources from the Kubernetes apiserver.
// It does not deserialize the objects into structured data, instead utilising
// the Kubernetes `Unstructured` type for data handling.
// This is to allow us to support arbitrary CRDs and resources that Preflight
// does not have registered as part of its `runtime.Scheme`.
type DataGathererDynamic struct {
	ctx context.Context
	// The 'dynamic' client used for fetching data.
	cl dynamic.Interface
	// groupVersionResource is the name of the API group, version and resource
	// that should be fetched by this data gatherer.
	groupVersionResource schema.GroupVersionResource
	// namespace, if specified, limits the namespace of the resources returned.
	// This field *must* be omitted when the groupVersionResource refers to a
	// non-namespaced resource.
	namespaces []string
	// fieldSelector is a field selector string used to filter resources
	// returned by the Kubernetes API.
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
	fieldSelector string
}

// Fetch will fetch the requested data from the apiserver, or return an error
// if fetching the data fails.
func (g *DataGathererDynamic) Fetch() (interface{}, error) {
	if g.groupVersionResource.Resource == "" {
		return nil, fmt.Errorf("resource type must be specified")
	}

	var list unstructured.UnstructuredList

	fetchNamespaces := g.namespaces
	if len(fetchNamespaces) == 0 {
		// then they must have been looking for all namespaces
		fetchNamespaces = []string{""}
	}

	for _, namespace := range fetchNamespaces {
		resourceInterface := namespaceResourceInterface(g.cl.Resource(g.groupVersionResource), namespace)
		namespaceList, err := resourceInterface.List(g.ctx, metav1.ListOptions{
			FieldSelector: g.fieldSelector,
		})
		if err != nil {
			if statusErr, ok := err.(*statusError.StatusError); ok {
				if statusErr.Status().Code == 404 {
					return nil, &dgerror.ConfigError{Err: err.Error()}
				}
			}
			return nil, err
		}
		list.Object = namespaceList.Object
		list.Items = append(list.Items, namespaceList.Items...)
	}

	// Redact Secret data
	err := redactList(&list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &list, nil
}

func redactList(list *unstructured.UnstructuredList) error {
	for i := range list.Items {
		// Determine the kind of items in case this is a generic 'mixed' list.
		gvks, _, err := scheme.Scheme.ObjectKinds(&list.Items[i])
		if err != nil {
			return errors.WithStack(err)
		}

		resource := list.Items[i]

		for _, gvk := range gvks {
			// If this item is a Secret then we need to redact it.
			if gvk.Kind == "Secret" && (gvk.Group == "core" || gvk.Group == "") {
				Select(SecretSelectedFields, &resource)

				// break when the object has been processed as a secret, no
				// other kinds have redact modifications
				break
			}

			// remove managedFields from all resources
			Redact([]string{
				"metadata.managedFields",
			}, &resource)
		}

		// update the object in the list
		list.Items[i] = resource
	}
	return nil
}

// namespaceResourceInterface will 'namespace' a NamespaceableResourceInterface
// if the 'namespace' parameter is non-empty, otherwise it will return the
// given ResourceInterface as-is.
func namespaceResourceInterface(iface dynamic.NamespaceableResourceInterface, namespace string) dynamic.ResourceInterface {
	if namespace == "" {
		return iface
	}
	return iface.Namespace(namespace)
}

// generateFieldSelector creates a field selector string from a list of
// namespaces to exclude.
func generateFieldSelector(excludeNamespaces []string) string {
	fieldSelector := fields.Nothing()
	for _, excludeNamespace := range excludeNamespaces {
		if excludeNamespace == "" {
			continue
		}
		fieldSelector = fields.AndSelectors(fields.OneTermNotEqualSelector("metadata.namespace", excludeNamespace), fieldSelector)
	}
	return fieldSelector.String()
}
