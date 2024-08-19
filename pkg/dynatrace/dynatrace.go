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

package dynatrace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
)

// ------------------------------synthetic-monitoring--------------------------
type DynatraceApiClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewDynatraceApiClient(baseURL, apiToken string) *DynatraceApiClient {
	return &DynatraceApiClient{
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
		EntityId      string `json:"entityId"`
	} `json:"locations"`
}

type ExistsHttpMonitorInDynatraceResponse struct {
	Monitors []struct {
		EntityId string `json:"entityId"`
	} `json:"monitors"`
}

// ------------------------------synthetic-monitoring--------------------------
// helper function to make Dynatrace api requests
func (dynatraceApiClient *DynatraceApiClient) MakeRequest(method, path string, renderedJSON string) (*http.Response, error) {
	url := dynatraceApiClient.baseURL + path
	var reqBody io.Reader
	if renderedJSON != "" {
		reqBody = bytes.NewBufferString(renderedJSON)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Api-Token "+dynatraceApiClient.apiToken)
	req.Header.Set("Content-Type", "application/json")

	return dynatraceApiClient.httpClient.Do(req)
}

func (dynatraceApiClient *DynatraceApiClient) GetDynatraceEquivalentClusterRegionId(clusterRegion string) (string, error) {
	// Adapted from spreadsheet in https://issues.redhat.com//browse/SDE-3754
	// Coming soon regions - il-central-1, ca-west-1
	awsRegionToDyntraceRegionMapping := map[string]string{
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

	// Look up the equivalent dynatrace location name based on the aws region in map
	//e.g. "us-east-2" in aws has equivalent "N. Virginia" in Dynatrace Locations
	dynatraceLocationName, ok := awsRegionToDyntraceRegionMapping[clusterRegion]
	if !ok {
		return "", fmt.Errorf("location not found for region: %s", clusterRegion)
	}

	//fetch dynatrace locations using dynatrace api
	resp, err := dynatraceApiClient.MakeRequest(http.MethodGet, "/synthetic/locations", "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch locations. Status code: %d", resp.StatusCode)
	}

	/*return location id from response body in which dynatrace location is public && CloudPlatform is AWS/AMAZON_EC2
	e.g. returns exampleLocationId for N. Virginia location in dynatrace in which CloudPlatform is AWS and location is public
	e.g. response body
	{
		"name": "N. Virginia",
		"entityId": "exampleLocationId",
		"type": "PUBLIC",
		"cloudPlatform": "AMAZON_EC2",
	}*/
	var locationResponse DynatraceLocation
	err = json.NewDecoder(resp.Body).Decode(&locationResponse)
	if err != nil {
		return "", err
	}
	for _, loc := range locationResponse.Locations {
		if loc.Name == dynatraceLocationName && loc.Type == "PUBLIC" && loc.CloudPlatform == "AMAZON_EC2" {
			return loc.EntityId, nil
		}
	}

	return "", fmt.Errorf("location '%s' not found", dynatraceLocationName)
}

func (dynatraceApiClient *DynatraceApiClient) CreateDynatraceHttpMonitor(monitorName, apiUrl, clusterId, dynatraceEquivalentClusterRegionId string) (string, error) {

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

	resp, err := dynatraceApiClient.MakeRequest(http.MethodPost, "/synthetic/monitors", renderedJSON)
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
		return "", fmt.Errorf("failed to fetch monitor id: %v", err)
	}
	monitorId := createdMonitor.EntityId
	return monitorId, nil
}

func (dynatraceApiClient *DynatraceApiClient) ExistsHttpMonitorInDynatrace(monitorId string) (bool, error) {
	path := ("/synthetic/monitors/")
	resp, err := dynatraceApiClient.MakeRequest(http.MethodGet, path, "")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Check if the response status code is OK
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to fetch monitor in Dynatrace. Status code: %d", resp.StatusCode)
	}

	var existsHttpMonitorResponse ExistsHttpMonitorInDynatraceResponse
	if err := json.NewDecoder(resp.Body).Decode(&existsHttpMonitorResponse); err != nil {
		return false, fmt.Errorf("error parsing JSON: %w", err)
	}

	for _, monitor := range existsHttpMonitorResponse.Monitors {
		if monitor.EntityId == monitorId {
			return true, nil
		}
	}
	return false, nil
}

func (dynatraceApiClient *DynatraceApiClient) DeleteDynatraceHttpMonitor(monitorId string) error {
	path := fmt.Sprintf("/synthetic/monitors/%s", monitorId)

	resp, err := dynatraceApiClient.MakeRequest(http.MethodDelete, path, "")
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
