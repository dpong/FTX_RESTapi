package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type RequestForLimitOrder struct {
	Market     string  `json:"market"`
	Side       string  `json:"side"`
	Price      float64 `json:"price"`
	Type       string  `json:"type"`
	Size       float64 `json:"size"`
	ReduceOnly bool    `json:"reduceOnly,omitempty"`
	Ioc        bool    `json:"ioc,omitempty"`
	PostOnly   bool    `json:"postOnly,omitempty"`
	ClientID   string  `json:"clientId,omitempty"`
}

type RequestForMarketOrder struct {
	Market     string  `json:"market"`
	Side       string  `json:"side"`
	Type       string  `json:"type"`
	Size       float64 `json:"size"`
	ReduceOnly bool    `json:"reduceOnly,omitempty"`
	Ioc        bool    `json:"ioc,omitempty"`
	PostOnly   bool    `json:"postOnly,omitempty"`
	ClientID   string  `json:"clientId,omitempty"`
}

type ResponseByOrder struct {
	Success bool `json:"success"`
	Result  struct {
		CreatedAt     time.Time `json:"createdAt"`
		FilledSize    float64   `json:"filledSize"`
		Future        string    `json:"future"`
		ID            int       `json:"id"`
		Market        string    `json:"market"`
		Price         float64   `json:"price"`
		RemainingSize float64   `json:"remainingSize"`
		Side          string    `json:"side"`
		Size          float64   `json:"size"`
		Status        string    `json:"status"`
		Type          string    `json:"type"`
		ReduceOnly    bool      `json:"reduceOnly"`
		Ioc           bool      `json:"ioc"`
		PostOnly      bool      `json:"postOnly"`
		ClientID      string    `json:"clientId"`
	} `json:"result"`
}

func (p *Client) PlaceLimitOrder(o *RequestForLimitOrder) (order *ResponseByOrder, err error) {
	body, err := json.Marshal(&o)
	if err != nil {
		return nil, err
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/orders",
		body, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &order)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (p *Client) PlaceMarketOrder(o *RequestForMarketOrder) (order *ResponseByOrder, err error) {
	body, err := json.Marshal(&o)
	if err != nil {
		return nil, err
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/orders",
		body, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &order)
	if err != nil {
		return nil, err
	}
	return order, nil
}

type ResponseByCancelOrder struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
}

type OnlySymbolOpt struct {
	Symbol string `json:"market,omitempty"`
}

func (p *Client) CancelAllOrders(symbol string) (result *ResponseByCancelOrder, err error) {
	var opt OnlySymbolOpt
	if symbol != "" {
		opt.Symbol = symbol
	}
	body, err := json.Marshal(&opt)
	if err != nil {
		return nil, err
	}
	res, err := p.sendRequest(
		http.MethodDelete,
		"/orders",
		body, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Client) CancelByID(oid int) (status *ResponseByCancelOrder, err error) {
	res, err := p.sendRequest(
		http.MethodDelete,
		fmt.Sprintf("/orders/%d", oid),
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

type ResponseByOrderStatus struct {
	Success bool `json:"success"`
	Result  struct {
		CreatedAt     string  `json:"createdAt"`
		FilledSize    float64 `json:"filledSize"`
		Future        string  `json:"future"`
		ID            int     `json:"id"`
		Market        string  `json:"market"`
		Price         float64 `json:"price"`
		AvgFillPrice  float64 `json:"avgFillPrice"`
		RemainingSize float64 `json:"remainingSize"`
		Side          string  `json:"side"`
		Size          float64 `json:"size"`
		Status        string  `json:"status"`
		Type          string  `json:"type"`
		ReduceOnly    bool    `json:"reduceOnly"`
		Ioc           bool    `json:"ioc"`
		PostOnly      bool    `json:"postOnly"`
		ClientID      string  `json:"clientId"`
	} `json:"result"`
}

func (p *Client) GetOrderStatus(oid int) (status *ResponseByOrderStatus, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		fmt.Sprintf("/orders/%d", oid),
		nil, nil)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

type GetOpenOrdersResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Createdat     time.Time `json:"createdAt"`
		Filledsize    float64   `json:"filledSize"`
		Future        string    `json:"future"`
		ID            int       `json:"id"`
		Market        string    `json:"market"`
		Price         float64   `json:"price"`
		Avgfillprice  float64   `json:"avgFillPrice"`
		Remainingsize float64   `json:"remainingSize"`
		Side          string    `json:"side"`
		Size          float64   `json:"size"`
		Status        string    `json:"status"`
		Type          string    `json:"type"`
		Reduceonly    bool      `json:"reduceOnly"`
		Ioc           bool      `json:"ioc"`
		Postonly      bool      `json:"postOnly"`
		Clientid      string    `json:"clientId,omitempty"`
	} `json:"result"`
}

func (p *Client) GetOpenOrders(symbol string) (result *GetOpenOrdersResponse, err error) {
	params := make(map[string]string)
	params["market"] = symbol
	res, err := p.sendRequest(
		http.MethodGet,
		"/orders",
		nil, &params)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type RequestForModifyOrder struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

type ModifyOrderResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Createdat     time.Time `json:"createdAt"`
		Filledsize    float64   `json:"filledSize"`
		Future        string    `json:"future"`
		ID            int       `json:"id"`
		Market        string    `json:"market"`
		Price         float64   `json:"price"`
		Remainingsize float64   `json:"remainingSize"`
		Side          string    `json:"side"`
		Size          float64   `json:"size"`
		Status        string    `json:"status"`
		Type          string    `json:"type"`
		Reduceonly    bool      `json:"reduceOnly"`
		Ioc           bool      `json:"ioc"`
		Postonly      bool      `json:"postOnly"`
		Clientid      string    `json:"clientId"`
	} `json:"result"`
}

func (p *Client) ModifyOrder(orderID int, o *RequestForModifyOrder) (order *ModifyOrderResponse, err error) {
	body, err := json.Marshal(&o)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	buffer.WriteString("/orders/")
	buffer.WriteString(strconv.Itoa(orderID))
	buffer.WriteString("/modify")
	res, err := p.sendRequest(
		http.MethodPost,
		buffer.String(),
		body, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &order)
	if err != nil {
		return nil, err
	}
	return order, nil
}
