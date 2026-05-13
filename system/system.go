package system

import (
	"encoding/json"
	"net/http"

	"github.com/umutyurdugull/GoFrame.PROD/core"
)

type InfoResponse struct {
	ZosmfVersion  string `json:"zosmf_version"`
	ZosVersion    string `json:"zos_version"`
	ZosmfHostname string `json:"zosmf_hostname"`
	ZosmfPort     string `json:"zosmf_port"`
	ApiVersion    string `json:"api_version"`
}

func GetInfo(client *core.Client) (*InfoResponse, error) {
	resp, err := client.Do(http.MethodGet, "/zosmf/info", nil, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}

	var info InfoResponse
	if err := json.Unmarshal(resp.Body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}
