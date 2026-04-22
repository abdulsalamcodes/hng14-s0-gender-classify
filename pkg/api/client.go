package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"hng14-s0-gender-classify/internal/models"
)

type Client struct {
	BaseURLs struct {
		Genderize   string
		Agify       string
		Nationalize string
	}
}

func NewClient(genderizeURL, agifyURL, nationalizeURL string) *Client {
	return &Client{
		BaseURLs: struct {
			Genderize   string
			Agify       string
			Nationalize string
		}{
			Genderize:   genderizeURL,
			Agify:       agifyURL,
			Nationalize: nationalizeURL,
		},
	}
}

func (c *Client) FetchGender(name string) (*models.GenderResponse, error) {
	var result models.GenderResponse
	if err := c.fetchJSON(c.BaseURLs.Genderize+"?name="+url.QueryEscape(name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) FetchAge(name string) (*models.AgeResponse, error) {
	var result models.AgeResponse
	if err := c.fetchJSON(c.BaseURLs.Agify+"?name="+url.QueryEscape(name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) FetchNationality(name string) (*models.NationalizeResponse, error) {
	var result models.NationalizeResponse
	if err := c.fetchJSON(c.BaseURLs.Nationalize+"?name="+url.QueryEscape(name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) fetchJSON(apiURL string, target interface{}) error {
	resp, err := http.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
