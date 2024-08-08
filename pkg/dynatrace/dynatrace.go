/*


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

package hostedcontrolplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
)

// ------------------------------synthetic-monitoring--------------------------
type DynatraceAPIClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewDynatraceAPIClient(baseURL, apiToken string) *DynatraceAPIClient {
	return &DynatraceAPIClient{
		baseURL:    baseURL,
		apiToken:   apiToken,
		httpClient: &http.Client{},
	}
}

var publicMonitorTemplate = `
{
    "name": "{{.MonitorName}}",
    "frequencyMin": 1,
    "enabled": true,
    "type": "HTTP",
    "script": {
        "version": "1.0",
        "requests": [
            {
                "description": "api availability",
                "url": "{{.ApiUrl}}",
                "method": "GET",
                "requestBody": "",
                "configuration": {
                    "acceptAnyCertificate": true,
                    "followRedirects": true
                },
                "preProcessingScript": "",
                "postProcessingScript": ""
            }
        ]
    },
    "locations": ["{{.DynatraceEquivalentClusterRegionId}}"],
    "anomalyDetection": {
        "outageHandling": {
            "globalOutage": true,
            "localOutage": false,
            "localOutagePolicy": {
                "affectedLocations": 1,
                "consecutiveRuns": 1
            }
        },
        "loadingTimeThresholds": {
            "enabled": true,
            "thresholds": [
                {
                    "type": "TOTAL",
                    "valueMs": 10000
                }
            ]
        }
    },
	"tags": [
        {
            "key": "cluster-id",
            "value": "{{.ClusterId}}"
        },
        {
            "key": "route-monitor-operator-managed",
            "value": "true"
        },
        {
            "key": "hcp-cluster",
            "value": "true"
        }
    ]
}
`

type DynatraceMonitorConfig struct {
	MonitorName                        string
	ApiUrl                             string
	DynatraceEquivalentClusterRegionId string
	ClusterId                          string
}

type DynatraceCreatedMonitor struct {
	EntityId string `json:"entityId"`
}

type DynatraceLocation struct {
	Locations []struct {
		Name          string `json:"name"`
		Type          string `json:"type"`
		CloudPlatform string `json:"cloudPlatform"`
		EntityID      string `json:"entityId"`
	} `json:"locations"`
}

// ------------------------------synthetic-monitoring--------------------------
// helper function to make Dynatrace api requests
func (DynatraceAPIClient *DynatraceAPIClient) MakeRequest(method, path string, renderedJSON string) (*http.Response, error) {
	url := DynatraceAPIClient.baseURL + path
	var reqBody io.Reader
	if renderedJSON != "" {
		reqBody = bytes.NewBufferString(renderedJSON)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Api-Token "+DynatraceAPIClient.apiToken)
	req.Header.Set("Content-Type", "application/json")

	return DynatraceAPIClient.httpClient.Do(req)
}

func (DynatraceAPIClient *DynatraceAPIClient) GetDynatraceEquivalentClusterRegionId(clusterRegion string) (string, error) {
	// Adapted from spreadsheet in https://issues.redhat.com//browse/SDE-3754
	// Coming soon regions - il-central-1, ca-west-1
	awsRegionToDyntraceLocationMapping := map[string]string{
		"us-east-1":      "N. Virginia",
		"us-east-2":      "N. Virginia",
		"us-west-1":      "Oregon",
		"us-west-2":      "Oregon",
		"af-south-1":     "São Paulo",
		"ap-southeast-1": "Singapore",
		"ap-southeast-2": "Sydney",
		"ap-southeast-3": "Singapore",
		"ap-southeast-4": "Sydney",
		"ap-northeast-1": "Singapore",
		"ap-northeast-2": "Sydney",
		"ap-northeast-3": "Singapore",
		"ap-south-1":     "Mumbai",
		"ap-south-2":     "Mumbai",
		"ap-east-1":      "Singapore",
		"ca-central-1":   "Montreal",
		"eu-west-1":      "Dublin",
		"eu-west-2":      "London",
		"eu-west-3":      "Frankfurt",
		"eu-central-1":   "Frankfurt",
		"eu-central-2":   "Frankfurt",
		"eu-south-1":     "Frankfurt",
		"eu-south-2":     "Frankfurt",
		"eu-north-1":     "London",
		"me-south-1":     "Mumbai",
		"me-central-1":   "Mumbai",
		"sa-east-1":      "São Paulo",
	}

	// Look up the dynatrace location name based on the aws region in map
	locationName, ok := awsRegionToDyntraceLocationMapping[clusterRegion]
	if !ok {
		return "", fmt.Errorf("location not found for region: %s", clusterRegion)
	}

	resp, err := DynatraceAPIClient.MakeRequest("GET", "/synthetic/locations", "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch locations. Status code: %d", resp.StatusCode)
	}

	//return location id from response body
	var locationResponse DynatraceLocation
	err = json.NewDecoder(resp.Body).Decode(&locationResponse)
	if err != nil {
		return "", err
	}
	for _, loc := range locationResponse.Locations {
		if loc.Name == locationName && loc.Type == "PUBLIC" && loc.CloudPlatform == "AMAZON_EC2" {
			return loc.EntityID, nil
		}
	}

	return "", fmt.Errorf("location '%s' not found", locationName)
}

func (DynatraceAPIClient *DynatraceAPIClient) CreateDynatraceHTTPMonitor(monitorName, apiUrl, clusterId, dynatraceEquivalentClusterRegionId string) (string, error) {

	tmpl := template.Must(template.New("jsonTemplate").Parse(publicMonitorTemplate))

	monitorConfig := DynatraceMonitorConfig{
		MonitorName:                        monitorName,
		ApiUrl:                             apiUrl,
		DynatraceEquivalentClusterRegionId: dynatraceEquivalentClusterRegionId,
		ClusterId:                          clusterId,
	}

	var tplBuffer bytes.Buffer
	err := tmpl.Execute(&tplBuffer, monitorConfig)
	if err != nil {
		return "", fmt.Errorf("error rendering JSON template - %v", err)
	}
	renderedJSON := tplBuffer.String()

	resp, err := DynatraceAPIClient.MakeRequest("POST", "/synthetic/monitors", renderedJSON)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create HTTP monitor. Status code: %d", resp.StatusCode)
	}

	//return monitor id
	var createdMonitor DynatraceCreatedMonitor
	err = json.NewDecoder(resp.Body).Decode(&createdMonitor)
	if err != nil {
		return "", fmt.Errorf("failed to fetch monitor id - %v", err)
	}
	monitorID := createdMonitor.EntityId
	return monitorID, nil
}

func (DynatraceAPIClient *DynatraceAPIClient) DeleteDynatraceHTTPMonitor(monitorID string) error {
	path := fmt.Sprintf("/synthetic/monitors/%s", monitorID)

	resp, err := DynatraceAPIClient.MakeRequest("DELETE", path, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	//monitor already deleted
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete monitor. Status code: %d", resp.StatusCode)
	}
	return nil
}
