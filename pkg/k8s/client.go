package k8s

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client wraps the Kubernetes client and provides methods to interact with the Kubernetes API
type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	namespace string
}

// NewClient creates a new Kubernetes client using the provided config flags
func NewClient(configFlags *genericclioptions.ConfigFlags) (*Client, error) {
	config, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	namespace, _, err := configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
		namespace: namespace,
	}, nil
}

// GetNamespace returns the current namespace
func (c *Client) GetNamespace() string {
	return c.namespace
}

// SetNamespace sets the current namespace
func (c *Client) SetNamespace(namespace string) {
	c.namespace = namespace
}

// GetConfig returns the REST config
func (c *Client) GetConfig() *rest.Config {
	return c.config
}

// GetClientset returns the Kubernetes clientset
func (c *Client) GetClientset() *kubernetes.Clientset {
	return c.clientset
}
