package datasets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/umutyurdugull/GoFrame.PROD/core"
)

type Dataset struct {
	Name   string `json:"dsname"`
	Dsorg  string `json:"dsorg"`
	RecFm  string `json:"recfm"`
	VolSer string `json:"volser"`
}

type DatasetListResponse struct {
	Items []Dataset `json:"items"`
}

type AllocateParams struct {
	Volser    string `json:"volser,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Dsorg     string `json:"dsorg,omitempty"`
	Alcunit   string `json:"alcunit,omitempty"`
	Primary   int    `json:"primary"`
	Secondary int    `json:"secondary"`
	Dirblk    int    `json:"dirblk,omitempty"`
	Avgblk    int    `json:"avgblk,omitempty"`
	Recfm     string `json:"recfm,omitempty"`
	Blksize   int    `json:"blksize,omitempty"`
	Lrecl     int    `json:"lrecl,omitempty"`
}

func List(client *core.Client, dsLevel string) ([]Dataset, error) {
	query := url.Values{}
	query.Set("dslevel", dsLevel)

	resp, err := client.Do(
		http.MethodGet,
		fmt.Sprintf("/zosmf/restfiles/ds?%s", query.Encode()),
		nil,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}

	var listResp DatasetListResponse
	if err := json.Unmarshal(resp.Body, &listResp); err != nil {
		return nil, err
	}

	return listResp.Items, nil
}

func Read(client *core.Client, dsName string) (string, error) {
	resp, err := client.Do(
		http.MethodGet,
		datasetPath(dsName),
		nil,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return "", err
	}

	return string(resp.Body), nil
}

func Write(client *core.Client, dsName string, content string) error {
	_, err := client.Do(
		http.MethodPut,
		datasetPath(dsName),
		bytes.NewBufferString(content),
		http.Header{"Content-Type": []string{"text/plain"}},
		http.StatusOK,
		http.StatusCreated,
		http.StatusNoContent,
	)
	return err
}

func Allocate(client *core.Client, dsName string, params AllocateParams) error {
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}

	_, err = client.Do(
		http.MethodPost,
		datasetPath(dsName),
		bytes.NewBuffer(payload),
		http.Header{"Content-Type": []string{"application/json"}},
		http.StatusCreated,
		http.StatusNoContent,
	)
	return err
}

func Delete(client *core.Client, dsName string) error {
	_, err := client.Do(
		http.MethodDelete,
		datasetPath(dsName),
		nil,
		nil,
		http.StatusNoContent,
	)
	return err
}

func datasetPath(dsName string) string {
	return fmt.Sprintf("/zosmf/restfiles/ds/%s", url.PathEscape(dsName))
}
