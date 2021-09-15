package api

import (
	"net/http"
)

type Account struct {
	Success bool `json:"success"`
	Result  struct {
		BackstopProvider             bool         `json:"backstopProvider"`
		Collateral                   float64      `json:"collateral"`
		FreeCollateral               float64      `json:"freeCollateral"`
		InitialMarginRequirement     float64      `json:"initialMarginRequirement"`
		Leverage                     float64      `json:"leverage"`
		Liquidating                  bool         `json:"liquidating"`
		MaintenanceMarginRequirement float64      `json:"maintenanceMarginRequirement"`
		MakerFee                     float64      `json:"makerFee"`
		MarginFraction               float64      `json:"marginFraction"`
		OpenMarginFraction           float64      `json:"openMarginFraction"`
		TakerFee                     float64      `json:"takerFee"`
		TotalAccountValue            float64      `json:"totalAccountValue"`
		TotalPositionSize            float64      `json:"totalPositionSize"`
		Username                     string       `json:"username"`
		Positions                    []*Positions `json:"positions"`
	} `json:"result"`
}

func (p *Client) Account() (account *Account, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/account",
		nil, nil)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &account)
	if err != nil {
		return nil, err
	}
	return account, nil
}

type ResponseByLeverageChanged struct {
	Result  interface{} `json:"result"`
	Success bool        `json:"success"`
}

func (p *Client) Leverage(n int) (leverage interface{}, err error) {
	params := make(map[string]interface{})
	params["leverage"] = n
	body, err := json.Marshal(params)
	if err != nil {
		return 0, err
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/account/leverage",
		body, nil)
	if err != nil {
		return 0, err
	}
	// in Close()
	err = decode(res, &leverage)
	if err != nil {
		return 0, err
	}
	return leverage, nil
}

type Balance struct {
	Success bool               `json:"success"`
	Result  []CoinsFromBalance `json:"result"`
}

type CoinsFromBalance struct {
	Coin                   string  `json:"coin"`
	Free                   float64 `json:"free"`
	Total                  float64 `json:"total"`
	AvailableWithoutBorrow float64 `json:"availableWithoutBorrow"`
	Borrowed               float64 `json:"spotBorrow"`
	USDValue               float64 `json:"usdValue"`
}

func (p *Client) Balances() (balances *Balance, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/wallet/balances",
		nil, nil)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &balances)
	if err != nil {
		return nil, err
	}
	return balances, nil
}
