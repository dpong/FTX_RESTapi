package api

import (
	"fmt"
	"net/http"
	"time"
)

type FillsResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Fee           float64   `json:"fee"`
		FeeRate       float64   `json:"feeRate"`
		Future        string    `json:"future"`
		ID            float64   `json:"id"`
		Liquidity     string    `json:"liquidity"`
		Market        string    `json:"market"`
		BaseCurrency  string    `json:"baseCurrency"`
		QuoteCurrency string    `json:"quoteCurrency"`
		OrderID       float64   `json:"orderId"`
		TradeID       float64   `json:"tradeId"`
		Price         float64   `json:"price"`
		Side          string    `json:"side"`
		Size          float64   `json:"size"`
		Time          time.Time `json:"time"`
		Type          string    `json:"type"`
	} `json:"result"`
}

func (p *Client) GetFills(symbol string, limit int) (fills *FillsResponse, err error) {
	params := make(map[string]string)
	params["market"] = symbol
	params["limit"] = fmt.Sprintf("%d", limit)
	res, err := p.sendRequest(
		http.MethodGet, "/fills",
		nil, &params)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &fills)
	if err != nil {
		return nil, err
	}
	return fills, nil
}
