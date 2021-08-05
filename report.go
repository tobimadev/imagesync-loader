package main

import (
	"encoding/json"
	"net/http"
)

type image struct {
	ID       int64  `json:"id"`
	Src      string `json:"src"`
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
	Done     bool   `json:"-"`
	Err      error  `json:"-"`
}

type product struct {
	ID       int64   `json:"id"`
	Handle   string  `json:"handle"`
	Title    string  `json:"title"`
	Vendor   string  `json:"vendor"`
	ProdType string  `json:"prodType"`
	Images   []image `json:"images"`
}

func (srv *server) readReport(url string) ([]*product, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	report := struct {
		Products []*product `json:"products"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, err
	}
	return report.Products, nil
}
