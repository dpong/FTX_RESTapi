package api

import (
	"fmt"
	"net/http"
	"time"
)

type RequestForOrder struct {
	Market     string      `json:"market"`
	Side       string      `json:"side"`
	Price      float64     `json:"price"`
	Type       string      `json:"type"`
	Size       float64     `json:"size"`
	ReduceOnly bool        `json:"reduceOnly,omitempty"` //沒有該選項的話會自動忽略
	Ioc        bool        `json:"ioc,omitempty"`
	PostOnly   bool        `json:"postOnly,omitempty"`
	ClientID   interface{} `json:"clientId,omitempty"`
}

type ResponseByOrder struct {
	Success bool `json:"success"`
	Result  struct {
		CreatedAt     time.Time   `json:"createdAt"`
		FilledSize    float64     `json:"filledSize"`
		Future        string      `json:"future"`
		ID            int         `json:"id"`
		Market        string      `json:"market"`
		Price         float64     `json:"price"`
		RemainingSize float64     `json:"remainingSize"`
		Side          string      `json:"side"`
		Size          float64     `json:"size"`
		Status        string      `json:"status"`
		Type          string      `json:"type"`
		ReduceOnly    bool        `json:"reduceOnly"`
		Ioc           bool        `json:"ioc"`
		PostOnly      bool        `json:"postOnly"`
		ClientID      interface{} `json:"clientId"`
	} `json:"result"`
}

func (p *Client) PlaceOrder(o *RequestForOrder) (order *ResponseByOrder) {
	body, err := json.Marshal(&o)
	if err != nil {
		p.Logger.Println(err)
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/orders",
		body, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &order)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return order
}

type ResponseByCancelOrder struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
}

func (p *Client) CancelAll() {
	//reader := bytes.NewBuffer(bytesData)
	_, err := p.sendRequest(
		http.MethodDelete,
		"/orders",
		nil, nil)
	if err != nil {
		p.Logger.Println(err)
		//return nil
	}
	// in Close()
	//err = decode(res, &status)
	//if err != nil {
	//	p.Logger.Println(err)
	//	return nil
	//}
	//return status
}

func (p *Client) CancelByID(oid int) (status *ResponseByCancelOrder) {
	res, err := p.sendRequest(
		http.MethodDelete,
		fmt.Sprintf("/orders/%d", oid),
		nil, nil)
	if err != nil {
		//p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &status)
	if err != nil {
		//p.Logger.Println(err)
		return nil
	}
	return status
}

type ResponseByOrderStatus struct {
	Success bool `json:"success"`
	Result  struct {
		CreatedAt     string      `json:"createdAt"`
		FilledSize    float64     `json:"filledSize"`
		Future        string      `json:"future"`
		ID            int         `json:"id"`
		Market        string      `json:"market"`
		Price         float64     `json:"price"`
		AvgFillPrice  float64     `json:"avgFillPrice"`
		RemainingSize float64     `json:"remainingSize"`
		Side          string      `json:"side"`
		Size          float64     `json:"size"`
		Status        string      `json:"status"`
		Type          string      `json:"type"`
		ReduceOnly    bool        `json:"reduceOnly"`
		Ioc           bool        `json:"ioc"`
		PostOnly      bool        `json:"postOnly"`
		ClientID      interface{} `json:"clientId"`
	} `json:"result"`
}

func (p *Client) GetOrderStatus(oid int) (status *ResponseByOrderStatus) {
	res, err := p.sendRequest(
		http.MethodGet,
		fmt.Sprintf("/orders/%d", oid),
		nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &status)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return status
}
