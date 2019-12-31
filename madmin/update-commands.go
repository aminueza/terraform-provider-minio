package madmin

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
)

// ServerUpdateStatus - contains the response of service update API
type ServerUpdateStatus struct {
	CurrentVersion string `json:"currentVersion"`
	UpdatedVersion string `json:"updatedVersion"`
}

// ServerUpdate - updates and restarts the MinIO cluster to latest version.
// optionally takes an input URL to specify a custom update binary link
func (adm *AdminClient) ServerUpdate(updateURL string) (us ServerUpdateStatus, err error) {
	queryValues := url.Values{}
	queryValues.Set("updateURL", updateURL)

	// Request API to Restart server
	resp, err := adm.executeMethod("POST", requestData{
		relPath:     adminAPIPrefix + "/update",
		queryValues: queryValues,
	})
	defer closeResponse(resp)
	if err != nil {
		return us, err
	}

	if resp.StatusCode != http.StatusOK {
		return us, httpRespToErrorResponse(resp)
	}

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return us, err
	}
	err = json.Unmarshal(buf, &us)
	return us, err
}
