package ui

import (
	"testing"

	"roeyazroel/kubectl-pfw/pkg/k8s"

	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestNewResourceFromService tests NewResourceFromService for correct Resource construction.
func TestNewResourceFromService(t *testing.T) {
	service := k8s.Service{
		Name:      "svc1",
		Namespace: "ns1",
		Ports: []k8s.ServicePort{
			{
				Port:           8080,
				Name:           "http",
				TargetPortSpec: &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
			},
		},
	}
	res := NewResourceFromService(service)
	assert.Equal(t, "svc1", res.Name)
	assert.Equal(t, "ns1", res.Namespace)
	assert.Equal(t, ServiceResource, res.Type)
	assert.Equal(t, []int32{8080}, res.Ports)
	assert.Equal(t, []string{"http"}, res.PortNames)
	assert.Equal(t, 1, len(res.TargetPortSpecs))
	assert.Equal(t, intstr.FromInt(80), *res.TargetPortSpecs[0])
}

// TestNewResourceFromPod tests NewResourceFromPod for correct Resource construction.
func TestNewResourceFromPod(t *testing.T) {
	pod := k8s.Pod{
		Name:      "pod1",
		Namespace: "ns1",
		Ports: []k8s.PodPort{
			{
				ContainerPort:   9090,
				Name:            "api",
				ContainerName:   "c1",
				IsInitContainer: false,
			},
		},
	}
	res := NewResourceFromPod(pod)
	assert.Equal(t, "pod1", res.Name)
	assert.Equal(t, "ns1", res.Namespace)
	assert.Equal(t, PodResource, res.Type)
	assert.Equal(t, []int32{9090}, res.Ports)
	assert.Equal(t, []string{"api"}, res.PortNames)
	assert.Equal(t, 1, len(res.TargetPortSpecs))
	assert.Equal(t, intstr.FromInt(9090), *res.TargetPortSpecs[0])
	assert.Equal(t, 1, len(res.PortMetadata))
	assert.Equal(t, "c1", res.PortMetadata[0].ContainerName)
	assert.False(t, res.PortMetadata[0].IsInitContainer)
}

// TestNewResourceFromDeployment tests NewResourceFromDeployment for correct Resource construction.
func TestNewResourceFromDeployment(t *testing.T) {
	dep := k8s.Deployment{
		Name:      "dep1",
		Namespace: "ns1",
	}
	res := NewResourceFromDeployment(dep)
	assert.Equal(t, "dep1", res.Name)
	assert.Equal(t, "ns1", res.Namespace)
	assert.Equal(t, DeploymentResource, res.Type)
	assert.Empty(t, res.Ports)
	assert.Empty(t, res.PortNames)
}

// TestNewResourceFromStatefulSet tests NewResourceFromStatefulSet for correct Resource construction.
func TestNewResourceFromStatefulSet(t *testing.T) {
	ss := k8s.StatefulSet{
		Name:      "ss1",
		Namespace: "ns1",
	}
	res := NewResourceFromStatefulSet(ss)
	assert.Equal(t, "ss1", res.Name)
	assert.Equal(t, "ns1", res.Namespace)
	assert.Equal(t, StatefulSetResource, res.Type)
	assert.Empty(t, res.Ports)
	assert.Empty(t, res.PortNames)
}

// mockAskOne is a helper to monkey-patch askOne for tests.
func mockAskOne(fn func(survey.Prompt, interface{}, ...survey.AskOpt) error) func() {
	orig := askOne
	askOne = fn
	return func() { askOne = orig }
}

// TestSelectResources_Success simulates a user selecting the first and third resources.
func TestSelectResources_Success(t *testing.T) {
	resources := []Resource{
		{Name: "a", DisplayName: "A"},
		{Name: "b", DisplayName: "B"},
		{Name: "c", DisplayName: "C"},
	}
	// Patch survey.AskOne to simulate user selecting 0 and 2
	restore := mockAskOne(func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		ptr, ok := response.(*[]int)
		if !ok {
			return errors.New("bad response type")
		}
		*ptr = []int{0, 2}
		return nil
	})
	defer restore()

	selected, err := SelectResources(resources, "pick")
	assert.NoError(t, err)
	assert.Equal(t, []Resource{resources[0], resources[2]}, selected)
}

// TestSelectResources_EmptyInput returns error if no resources.
func TestSelectResources_EmptyInput(t *testing.T) {
	selected, err := SelectResources([]Resource{}, "pick")
	assert.Error(t, err)
	assert.Nil(t, selected)
}

// TestSelectResources_UserCancels simulates user canceling selection.
func TestSelectResources_UserCancels(t *testing.T) {
	resources := []Resource{{Name: "a", DisplayName: "A"}}
	restore := mockAskOne(func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		return errors.New("user canceled")
	})
	defer restore()

	selected, err := SelectResources(resources, "pick")
	assert.Error(t, err)
	assert.Nil(t, selected)
}

// TestSelectResources_NoSelection simulates user making no selection.
func TestSelectResources_NoSelection(t *testing.T) {
	resources := []Resource{{Name: "a", DisplayName: "A"}}
	restore := mockAskOne(func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		ptr, ok := response.(*[]int)
		if !ok {
			return errors.New("bad response type")
		}
		*ptr = []int{} // user selects nothing
		return nil
	})
	defer restore()

	selected, err := SelectResources(resources, "pick")
	assert.Error(t, err)
	assert.Nil(t, selected)
}

// TestAskForLocalPort_Success simulates user entering a valid port.
func TestAskForLocalPort_Success(t *testing.T) {
	resource := Resource{Name: "svc", PortNames: []string{"http"}}
	restore := mockAskOne(func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		ptr, ok := response.(*string)
		if !ok {
			return errors.New("bad response type")
		}
		*ptr = "12345"
		return nil
	})
	defer restore()

	port, err := AskForLocalPort(resource, 8080, 0)
	assert.NoError(t, err)
	assert.Equal(t, int32(12345), port)
}

// TestAskForLocalPort_InvalidInput simulates user entering an invalid port.
func TestAskForLocalPort_InvalidInput(t *testing.T) {
	resource := Resource{Name: "svc", PortNames: []string{"http"}}
	calls := 0
	restore := mockAskOne(func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		calls++
		ptr, ok := response.(*string)
		if !ok {
			return errors.New("bad response type")
		}
		if calls == 1 {
			*ptr = "notaport"
			return errors.New("please enter a valid port number (1-65535)")
		}
		*ptr = "65536"
		return errors.New("please enter a valid port number (1-65535)")
	})
	defer restore()

	_, err := AskForLocalPort(resource, 8080, 0)
	assert.Error(t, err)
}
