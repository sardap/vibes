package vibes

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

//Invoker used to invoke the api endpoints
type Invoker struct {
	Endpoint  string
	AccessKey string
	Scheme    string
	Username  string
	Password  string
}

//GetSets returns sets from server
func (i *Invoker) GetSets() ([]string, error) {
	url := url.URL{
		Scheme: i.Scheme,
		Host:   i.Endpoint,
		Path:   "api/get_set",
	}
	q := url.Query()
	q.Set("access_key", i.AccessKey)
	url.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(i.Username, i.Password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(
			err,
			fmt.Sprintf("unable to fetch %s", url.String()),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Unable to fetch %s body %s", url.String(), string(bodyBytes))
	}

	var data []string
	err = json.NewDecoder(resp.Body).Decode(&data)

	return data, err

}

type sampleLengthResult struct {
	LengthMS float64 `json:"length_ms"`
}

//GetSampleLength returns sample length
func (i *Invoker) GetSampleLength() (time.Duration, error) {
	url := url.URL{
		Scheme: i.Scheme,
		Host:   i.Endpoint,
		Path:   "api/get_sample_length",
	}
	q := url.Query()
	q.Set("access_key", i.AccessKey)
	url.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(i.Username, i.Password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.Wrap(
			err,
			fmt.Sprintf("unable to fetch %s", url.String()),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("Unable to fetch %s body %s", url.String(), string(bodyBytes))
	}

	var data sampleLengthResult
	err = json.NewDecoder(resp.Body).Decode(&data)

	return time.Duration(data.LengthMS) * time.Millisecond, err

}

//GetBellStream returns bell sound stream from server
func (i *Invoker) GetBellStream() (io.ReadCloser, error) {
	url := url.URL{
		Scheme: i.Scheme, Host: i.Endpoint, Path: "api/get_bell",
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(i.Username, i.Password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(
			err,
			fmt.Sprintf("unable to fetch %s", url.String()),
		)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Unable to fetch %s body %s", url.String(), string(bodyBytes))
	}

	return resp.Body, err
}

//GetSampleStream returns sample stream from server
func (i *Invoker) GetSampleStream(hour int, set, city, country string) (io.ReadCloser, error) {
	fmt.Printf("Getting Set:%s Hour:%d\n", set, hour)
	path := fmt.Sprintf("api/get_sample/%s/%s/%s/%d", country, city, set, hour)
	url := url.URL{
		Scheme: i.Scheme, Host: i.Endpoint, Path: path,
	}
	q := url.Query()
	q.Set("access_key", i.AccessKey)
	url.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(i.Username, i.Password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(
			err,
			fmt.Sprintf("unable to fetch %s", url.String()),
		)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Unable to fetch %s body %s", url.String(), string(bodyBytes))
	}

	return resp.Body, err
}
