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
	"strings"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
)

const (
	httpMonitorPath = "/synthetic/monitors"
	locationPath    = "/synthetic/locations"
)

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
            "key": "cluster-region",
            "value": "{{.ClusterRegion}}"
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

// BasicHttpMonitor defines the minimal HTTP monitor object returned when using certain HTTP methods (ie - list)
// See https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-api/environment-api/synthetic/synthetic-monitors/get-all-monitors
// for more info
type BasicHttpMonitor struct {
	Enabled  bool   `json:"enabled,omitempty"`
	EntityId string `json:"entityId,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
}

// HttpMonitor is the fully defined HTTP monitor object in Dynatrace
//
// NOTE: this structure does not contain all possible fields - only the ones needed for this package. See
// https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-api/environment-api/synthetic/synthetic-monitors/models#expand--httpsyntheticmonitor--2
// for more information
type HttpMonitor struct {
	BasicHttpMonitor
	Locations []string `json:"locations,omitempty"`
}

type DynatraceMonitorConfig struct {
	MonitorName                        string
	ApiUrl                             string
	DynatraceEquivalentClusterRegionId string
	ClusterId                          string
	ClusterRegion                      string
}

type DynatraceLocation struct {
	Locations []struct {
		Name          string `json:"name"`
		Type          string `json:"type"`
		CloudPlatform string `json:"cloudPlatform"`
		EntityId      string `json:"entityId"`
		Status        string `json:"status"`
	} `json:"locations"`
}

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

func (dynatraceApiClient *DynatraceApiClient) ListDynatraceHttpMonitorsForCluster(clusterId string) ([]BasicHttpMonitor, error) {
	path := fmt.Sprintf("%s/?tag=cluster-id:%s", httpMonitorPath, clusterId)
	resp, err := dynatraceApiClient.MakeRequest(http.MethodGet, path, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch monitor in Dynatrace. Status code: %d", resp.StatusCode)
	}

	response := struct {
		Monitors []BasicHttpMonitor `json:"monitors,omitempty"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	return response.Monitors, nil
}

func (dynatraceApiClient *DynatraceApiClient) GetDynatraceHttpMonitor(entityId string) (HttpMonitor, error) {
	path := fmt.Sprintf("%s/%s", httpMonitorPath, entityId)
	resp, err := dynatraceApiClient.MakeRequest(http.MethodGet, path, "")
	if err != nil {
		return HttpMonitor{}, fmt.Errorf("failed to GET HTTP monitor from Dynatrace: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return HttpMonitor{}, fmt.Errorf("unexpected response from Dynatrace when retrieving monitor %q: expected status code %d, got %d.\nFull response: %#v", entityId, http.StatusOK, resp.StatusCode, resp)
	}

	monitor := HttpMonitor{}
	err = json.NewDecoder(resp.Body).Decode(&monitor)
	if err != nil {
		return HttpMonitor{}, fmt.Errorf("failed to decode response body: %w", err)
	}
	return monitor, nil
}

func (dynatraceApiClient *DynatraceApiClient) GetLocationEntityIdFromDynatrace(locationName string, locationType hypershiftv1beta1.AWSEndpointAccessType) (string, error) {
	// Fetch Dynatrace locations using Dynatrace API
	resp, err := dynatraceApiClient.MakeRequest(http.MethodGet, locationPath, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch locations. Status code: %d", resp.StatusCode)
	}

	/*return location id from response body in which dynatrace location is public && CloudPlatform is AWS/AMAZON_EC2
	e.g. returns exampleLocationId for N. Virginia location in dynatrace in which CloudPlatform is AWS and location is public
	e.g. PublicAndPrivate response body
	{
		"name": "N. Virginia",
		"entityId": "exampleLocationId",
		"type": "PUBLIC",
		"cloudPlatform": "AMAZON_EC2",
		"status": "ENABLED"
	}*/

	/*
		e.g. Private Response body
		{
			"name": "backplanei03xyz",
			"entityId": "privateLocationId",
			"type": "PRIVATE",
			"status": "ENABLED"
		},
	*/
	// Decode the response body
	var locationResponse DynatraceLocation
	err = json.NewDecoder(resp.Body).Decode(&locationResponse)
	if err != nil {
		return "", err
	}

	if locationType == hypershiftv1beta1.PublicAndPrivate {
		for _, loc := range locationResponse.Locations {
			if loc.Name == locationName && loc.Type == "PUBLIC" && loc.CloudPlatform == "AMAZON_EC2" && loc.Status == "ENABLED" {
				return loc.EntityId, nil
			}
		}
	}
	if locationType == hypershiftv1beta1.Private {
		for _, loc := range locationResponse.Locations {
			if strings.Contains(loc.Name, locationName) && loc.Type == "PRIVATE" && loc.Status == "ENABLED" {
				return loc.EntityId, nil
			}
		}
	}

	return "", fmt.Errorf("location '%s' not found for location type '%s'", locationName, locationType)
}

// CreateDynatraceHttpMonitor creates a new HTTP monitor in dynatrace, returning the resulting monitor's EntityId
func (dynatraceApiClient *DynatraceApiClient) CreateDynatraceHttpMonitor(monitorName, apiUrl, clusterId, dynatraceEquivalentClusterRegionId, clusterRegion string) (string, error) {

	tmpl := template.Must(template.New("jsonTemplate").Parse(publicMonitorTemplate))

	monitorConfig := DynatraceMonitorConfig{
		MonitorName:                        monitorName,
		ApiUrl:                             apiUrl,
		DynatraceEquivalentClusterRegionId: dynatraceEquivalentClusterRegionId,
		ClusterId:                          clusterId,
		ClusterRegion:                      clusterRegion,
	}

	var tplBuffer bytes.Buffer
	err := tmpl.Execute(&tplBuffer, monitorConfig)
	if err != nil {
		return "", fmt.Errorf("error rendering JSON template - %v", err)
	}
	renderedJSON := tplBuffer.String()

	resp, err := dynatraceApiClient.MakeRequest(http.MethodPost, httpMonitorPath, renderedJSON)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read response body for error details
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("failed to create HTTP monitor. Status code: %d (unable to read response body: %v)", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("failed to create HTTP monitor. Status code: %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	//return monitor id
	var createdMonitor BasicHttpMonitor
	err = json.NewDecoder(resp.Body).Decode(&createdMonitor)
	if err != nil {
		return "", fmt.Errorf("failed to fetch monitor id: %v", err)
	}
	monitorId := createdMonitor.EntityId
	return monitorId, nil
}

func (dynatraceApiClient *DynatraceApiClient) DeleteSingleMonitor(monitorId string) error {
	path := fmt.Sprintf("%s/%s", httpMonitorPath, monitorId)
	resp, err := dynatraceApiClient.MakeRequest(http.MethodDelete, path, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete monitor %s. Status code: %d", monitorId, resp.StatusCode)
	}
	return nil
}

func (dynatraceApiClient *DynatraceApiClient) DeleteDynatraceMonitorByCluserId(clusterId string) error {
	monitors, err := dynatraceApiClient.ListDynatraceHttpMonitorsForCluster(clusterId)
	if err != nil {
		return err
	}

	for _, monitor := range monitors {
		err := dynatraceApiClient.DeleteSingleMonitor(monitor.EntityId)
		if err != nil {
			return err
		}
	}
	return nil
}
