package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type StreamUserTradesBranch struct {
	cancel     *context.CancelFunc
	conn       *websocket.Conn
	key        string
	secret     string
	subaccount string

	tradeSets tradeDataMap
	logger    *logrus.Logger
}

type UserTradeData struct {
	Symbol    string
	Side      string
	Oid       string
	OrderType string
	IsMaker   bool
	Price     decimal.Decimal
	Qty       decimal.Decimal
	Fee       decimal.Decimal
	TimeStamp time.Time
}

type tradeDataMap struct {
	mux sync.RWMutex
	set map[string][]UserTradeData
}

// func (c *Client) UserTradeStream(logger *logrus.Logger) *StreamUserTradesBranch {
// 	return c.userTradeStream(logger)
// }

func (c *Client) CloseUserTradeStream() {
	(*c.userTrade.cancel)()
}

func (c *Client) InitUserTradeStream(logger *log.Logger) {
	c.userTradeStream(logger)
}

// err is no trade set
func (c *Client) ReadUserTradeWithSymbol(symbol string) ([]UserTradeData, error) {
	uSymbol := strings.ToUpper(symbol)
	c.userTrade.tradeSets.mux.Lock()
	defer c.userTrade.tradeSets.mux.Unlock()
	var result []UserTradeData
	if data, ok := c.userTrade.tradeSets.set[uSymbol]; !ok {
		return data, errors.New("no trade set can be requested")
	} else {
		new := []UserTradeData{}
		result = data
		c.userTrade.tradeSets.set[uSymbol] = new
	}
	return result, nil
}

// err is no trade
// mix up with multiple symbol's trade data
func (c *Client) ReadUserTrade() ([]UserTradeData, error) {
	c.userTrade.tradeSets.mux.Lock()
	defer c.userTrade.tradeSets.mux.Unlock()
	var result []UserTradeData
	for key, item := range c.userTrade.tradeSets.set {
		// each symbol
		result = append(result, item...)
		// earse old data
		new := []UserTradeData{}
		c.userTrade.tradeSets.set[key] = new
	}
	if len(result) == 0 {
		return result, errors.New("no trade data")
	}
	return result, nil
}

func (c *Client) userTradeStream(logger *logrus.Logger) {
	o := new(StreamUserTradesBranch)
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = &cancel
	o.key = c.key
	o.secret = c.secret
	o.subaccount = c.subaccount

	o.tradeSets.set = make(map[string][]UserTradeData, 5)
	o.logger = logger
	go o.maintainSession(ctx)
	c.userTrade = o
}

func (u *StreamUserTradesBranch) insertTrade(input *UserTradeData) {
	u.tradeSets.mux.Lock()
	defer u.tradeSets.mux.Unlock()
	if _, ok := u.tradeSets.set[input.Symbol]; !ok {
		// not in the map yet
		data := []UserTradeData{*input}
		u.tradeSets.set[input.Symbol] = data
	} else {
		// already in the map
		data := u.tradeSets.set[input.Symbol]
		data = append(data, *input)
		u.tradeSets.set[input.Symbol] = data
	}
}

func (o *StreamUserTradesBranch) maintainSession(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := o.maintain(ctx); err == nil {
				return
			} else {
				o.logger.Warningf("reconnect FTX user trade stream with err: %s\n", err.Error())
			}
		}
	}
}

func (o *StreamUserTradesBranch) maintain(ctx context.Context) error {
	var duration time.Duration = 30
	innerErr := make(chan error, 1)
	url := "wss://ftx.com/ws/"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	o.conn = conn
	defer o.conn.Close()
	send := o.getAuthMessage(o.key, o.secret, o.subaccount)
	if err := o.conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	send = o.getSubscribeMessage("fills", "")
	if err := o.conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	if err := o.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	o.conn.SetPingHandler(nil)
	go func() {
		PingManaging := time.NewTicker(time.Second * 15)
		defer PingManaging.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-innerErr:
				return
			case <-PingManaging.C:
				send := getPingPong()
				if err := o.conn.WriteMessage(websocket.TextMessage, send); err != nil {
					o.conn.SetReadDeadline(time.Now().Add(time.Second))
					return
				}
				o.conn.SetReadDeadline(time.Now().Add(time.Second * duration))
			default:
				time.Sleep(time.Second)
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, msg, err := o.conn.ReadMessage()
			if err != nil {
				innerErr <- errors.New("restart")
				return err
			}
			res, err1 := o.decoding(msg)
			if err1 != nil {
				innerErr <- errors.New("restart")
				return err1
			}
			err2 := o.handleFTXWebsocket(res)
			if err2 != nil {
				innerErr <- errors.New("restart")
				return err2
			}
			if err := o.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
				innerErr <- errors.New("restart")
				return err
			}
		} // end select
	} // end for
}

type fTXAuthenticationMessage struct {
	Args argsN  `json:"args"`
	Op   string `json:"op"`
}

func (t *StreamUserTradesBranch) getAuthMessage(key string, secret string, subaccount string) []byte {
	nonce := time.Now().UnixNano() / int64(time.Millisecond)
	req := fmt.Sprintf("%dwebsocket_login", nonce)
	sig := hmac.New(sha256.New, []byte(secret))
	sig.Write([]byte(req))
	signature := hex.EncodeToString(sig.Sum(nil))
	auth := fTXAuthenticationMessage{Op: "login", Args: argsN{Key: key, Sign: signature, Time: nonce, Subaccount: subaccount}}
	message, err := json.Marshal(auth)
	if err != nil {
		log.Println(err)
	}
	return message
}

func (t *StreamUserTradesBranch) getSubscribeMessage(channel, market string) []byte {
	sub := fTXSubscribeMessage{Op: "subscribe", Channel: channel, Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		log.Println(err)
	}
	return message
}

func (t *StreamUserTradesBranch) decoding(message []byte) (res map[string]interface{}, err error) {
	if message == nil {
		err = errors.New("the incoming message is nil")
		return nil, err
	}
	err = json.Unmarshal(message, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (t *StreamUserTradesBranch) handleFTXWebsocket(res map[string]interface{}) error {
	switch {
	case res["type"] == "error":
		Msg := res["msg"].(string)
		Code := res["code"].(float64)
		sCode := strconv.FormatFloat(Code, 'f', -1, 64)
		var buffer bytes.Buffer
		buffer.WriteString("Code: ")
		buffer.WriteString(sCode)
		buffer.WriteString(" | ")
		buffer.WriteString(Msg)
		err := errors.New(buffer.String())
		return err
	case res["type"] == "subscribed":
		Channel := res["channel"].(string)
		var buffer bytes.Buffer
		buffer.WriteString("Sub | FTX channel: ")
		buffer.WriteString(Channel)
		t.logger.Infoln(buffer.String())
		return nil
	case res["type"] == "info":
		Code := res["code"].(float64)
		if Code == 20001 {
			err := errors.New("server restart, code 20001。")
			return err
		}
	case res["type"] == "update":
		Channel := res["channel"].(string)
		if Channel == "fills" {
			resp, ok := res["data"].(map[string]interface{})
			if !ok {
				return errors.New("fail to get data")
			}
			var exc decimal.Decimal
			if fsize, ok := resp["size"].(float64); !ok {
				return errors.New("fail to get size")
			} else {
				exc = decimal.NewFromFloat(fsize)
			}
			if exc.IsZero() {
				return errors.New("exc is zero")
			}
			var symbol string
			if sym, ok := resp["market"].(string); !ok {
				return errors.New("fail to get symbol")
			} else {
				symbol = sym
			}
			var oid string
			if id, ok := resp["orderId"].(float64); !ok {
				return errors.New("fail to get oid")
			} else {
				oid = decimal.NewFromFloat(id).String()
			}
			var price decimal.Decimal
			if fprice, ok := resp["price"].(float64); !ok {
				return errors.New("fail to get price")
			} else {
				price = decimal.NewFromFloat(fprice)
			}
			var liq string
			if ll, ok := resp["liquidity"].(string); !ok {
				return errors.New("fail to get liq")
			} else {
				liq = ll
			}
			var fee decimal.Decimal
			if ffee, ok := resp["fee"].(float64); !ok {
				return errors.New("fail to get fee")
			} else {
				fee = decimal.NewFromFloat(ffee)
			}
			var side string
			if s, ok := resp["side"].(string); !ok {
				return errors.New("fail to get side")
			} else {
				side = s
			}
			var orderType string
			if o, ok := resp["type"].(string); !ok {
				return errors.New("fail to get side")
			} else {
				orderType = o
			}
			var st time.Time
			if s, ok := resp["time"].(string); !ok {
				return errors.New("fail to get time")
			} else {
				stamp, err := time.Parse(timeLayout, s)
				if err != nil {
					return errors.New("fail to parse time")
				}
				st = stamp
			}
			out := new(UserTradeData)
			out.Fee = fee
			out.Qty = exc
			out.Price = price
			out.Oid = oid
			out.Side = side
			out.OrderType = orderType
			if liq == "maker" {
				out.IsMaker = true
			} else {
				out.IsMaker = false
			}
			out.Symbol = symbol
			out.TimeStamp = st
			t.insertTrade(out)
			return nil
		}
	default:
	}
	return nil
}

func getPingPong() []byte {
	sub := fTXSubscribeMessage{Op: "ping"}
	message, err := json.Marshal(sub)
	if err != nil {
		log.Println(err)
	}
	return message
}
