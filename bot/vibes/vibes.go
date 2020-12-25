package vibes

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
)

//Invoker used to invoke the api endpoints
type Invoker struct {
	Endpoint  string
	AccessKey string
}

//GetSets returns sets from server
func (i *Invoker) GetSets() ([]string, error) {
	url := url.URL{
		Scheme: "https",
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

//GetSample returns sets from server
func (i *Invoker) GetSample(hour int, set, city, country string) ([]byte, error) {
	path := fmt.Sprintf("api/get_sample/%s/%d", set, hour)
	url := url.URL{
		Scheme: "https", Host: i.Endpoint, Path: path,
	}
	q := url.Query()
	q.Set("access_key", i.AccessKey)
	q.Set("city_name", city)
	q.Set("country_code", country)
	url.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

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

	bodyBytes, err := ioutil.ReadAll(resp.Body)

	return bodyBytes, err

}
