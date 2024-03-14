package sunfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/IBM/composable-resource-operator/api/v1alpha1"
	"net/http"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	setupLog = ctrl.Log.WithName("sunfish_client")
)

type ProcessorType string

const (
	TypeGpu = "GPU"
)

type SupportedGPU string

const (
	V100    = "Tesla-V100-PCIE-16GB"
	A10040G = "NVIDIA-A100-PCIE-40GB"
	A10080G = "NVIDIA-A100-80GB-PCIe"
)

type CompositionRequest struct {
	Name  string     `json:"Name"`
	Procs Processors `json:"Processors"`
}

type Processors struct {
	Members []ProcessorRequest `json:"Members"`
}

type ProcessorRequest struct {
	RequestCount int64  `json:"@Redfish.RequestCount"`
	ProcType     string `json:"ProcessorType"`
	Model        string `json:"Model"`
}

type SunfishClient struct {
	compositionServiceEndpoint string
}

func NewSunfishClient() *SunfishClient {

	endpoint := "composition-service.cro-system.svc.cluster.local:5060"
	if ep := os.Getenv("SUNFISH_ENDPOINT"); ep != "" {
		endpoint = ep
	}

	return &SunfishClient{compositionServiceEndpoint: endpoint}
}

func (s *SunfishClient) sendPatchRequest(cr CompositionRequest) error {
	crJson, _ := json.Marshal(cr)
	setupLog.Info("requesting resources attachment", "json_request", string(crJson))

	bodyReader := bytes.NewReader(crJson)

	//req, err := http.NewRequest("PATCH", "http://127.0.0.1:5060/redfish/v1/Systems/System", bodyReader)
	req, err := http.NewRequest("PATCH", "http://"+s.compositionServiceEndpoint+"/redfish/v1/Systems/System", bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("http returned code %d", response.StatusCode)
	}

	return nil
}

func (s *SunfishClient) AddResource(instance *v1alpha1.ComposabilityRequest) error {

	pr := ProcessorRequest{}

	switch instance.Spec.Resources.ScalarResources["gpu"].Model {
	case V100, A10080G, A10040G:
		pr.Model = instance.Spec.Resources.ScalarResources["gpu"].Model
		pr.ProcType = TypeGpu
		pr.RequestCount = instance.Spec.Resources.ScalarResources["gpu"].Size
	}

	prs := Processors{Members: []ProcessorRequest{pr}}

	cr := CompositionRequest{Name: instance.Spec.TargetNode, Procs: prs}

	return s.sendPatchRequest(cr)

}

func (s *SunfishClient) RemoveResource(instance *v1alpha1.ComposabilityRequest) error {

	pr := ProcessorRequest{}
	switch instance.Spec.Resources.ScalarResources["gpu"].Model {
	case V100, A10080G, A10040G:
		pr.Model = instance.Spec.Resources.ScalarResources["gpu"].Model
		pr.ProcType = TypeGpu
		pr.RequestCount = 0
	}

	prs := Processors{Members: []ProcessorRequest{pr}}

	cr := CompositionRequest{Name: instance.Spec.TargetNode, Procs: prs}

	return s.sendPatchRequest(cr)

}
