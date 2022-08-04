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

var duration time.Duration = 20

type StreamUserTradesBranch struct {
	cancel     *context.CancelFunc
	conn       *websocket.Conn
	key        string
	secret     string
	subaccount string

	subscribeCheck subscribeCheck

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

type subscribeCheck struct {
	subTime time.Time
	done    bool
}

func (c *Client) CloseUserTradeStream() {
	(*c.userTrade.cancel)()
}

func (c *Client) InitUserTradeStream(logger *log.Logger) {
	c.userTradeStream(logger)
}

// err is no trade set
func (c *Client) ReadUserTradeWithSymbol(symbol string) ([]UserTradeData, error) {
	c.userTrade.tradeSets.mux.Lock()
	defer c.userTrade.tradeSets.mux.Unlock()
	uSymbol := strings.ToUpper(symbol)
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
	go c.maintainSession(ctx)
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

func (c *Client) maintainSession(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := c.maintain(ctx); err == nil {
				return
			} else {
				c.beforeFillsSnapShot()
				c.userTrade.logger.Warningf("reconnect FTX user trade stream with err: %s\n", err.Error())
			}
		}
	}
}

func (c *Client) maintain(ctx context.Context) error {
	innerErr := make(chan error, 1)
	url := "wss://ftx.com/ws/"
	// wait 5 second, if the hand shake fail, will terminate the dail
	dailCtx, _ := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	conn, _, err := websocket.DefaultDialer.DialContext(dailCtx, url, nil)
	if err != nil {
		return err
	}
	c.userTrade.conn = conn
	defer c.userTrade.conn.Close()
	// make sure subscribe is done in time
	c.userTrade.subscribeCheck.subTime = time.Now()
	send := c.userTrade.getAuthMessage(c.userTrade.key, c.userTrade.secret, c.userTrade.subaccount)
	if err := c.userTrade.conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	send = c.userTrade.getSubscribeMessage("fills", "")
	if err := c.userTrade.conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	if err := c.userTrade.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	// detecting missing fills
	c.afterFillsSnapShot()

	c.userTrade.conn.SetPingHandler(nil)
	go func() {
		PingManaging := time.NewTicker(time.Second * 10)
		defer PingManaging.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-innerErr:
				return
			case <-PingManaging.C:
				send := getPingPong()
				if err := c.userTrade.conn.WriteMessage(websocket.TextMessage, send); err != nil {
					c.userTrade.conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500))
					return
				}
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
			if c.userTrade.conn == nil {
				message := "FTX private channel disconnected..."
				innerErr <- errors.New("restart")
				return errors.New(message)
			}
			_, msg, err := c.userTrade.conn.ReadMessage()
			if err != nil {
				innerErr <- errors.New("restart")
				return err
			}
			res, err1 := c.userTrade.decoding(msg)
			if err1 != nil {
				innerErr <- errors.New("restart")
				return err1
			}
			err2 := c.userTrade.handleFTXWebsocket(res)
			if err2 != nil {
				innerErr <- errors.New("restart")
				return err2
			}
			if err := c.userTrade.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
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
	if !t.subscribeCheck.done {
		if time.Now().After(t.subscribeCheck.subTime.Add(time.Second * 10)) {
			return errors.New("fail to subscribe in 10s")
		}
	}
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
		// subscribe done
		t.subscribeCheck.done = true
		Channel := res["channel"].(string)
		var buffer bytes.Buffer
		buffer.WriteString("Sub | FTX private channel: ")
		buffer.WriteString(Channel)
		t.logger.Infoln(buffer.String())
	case res["type"] == "info":
		Code := res["code"].(float64)
		if Code == 20001 {
			err := errors.New("server restart, code 20001。")
			return err
		}
	case res["type"] == "pong":
		// extend
		if err := t.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
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

// missing fills detecting

type fillsDetecting struct {
	before *FillsResponse
	after  *FillsResponse
}

func (c *Client) beforeFillsSnapShot() error {
	if res, err := c.GetFills("", 100); err != nil {
		return err
	} else {
		c.fillDetect.before = res
	}
	return nil
}

func (c *Client) afterFillsSnapShot() error {
	if c.fillDetect.before == nil {
		return nil
	}
	if res, err := c.GetFills("", 100); err != nil {
		return err
	} else {
		c.fillDetect.after = res
	}
	for _, trade := range c.fillDetect.after.Result {
		if c.thisTradeIdWeHaveIt(trade.ID) {
			continue
		}
		// missing fill detected
		out := new(UserTradeData)
		out.Fee = decimal.NewFromFloat(trade.Fee)
		out.Qty = decimal.NewFromFloat(trade.Size)
		out.Price = decimal.NewFromFloat(trade.Price)
		out.Oid = decimal.NewFromFloat(trade.OrderID).String()
		out.Side = trade.Side
		out.OrderType = trade.Type
		if trade.Liquidity == "maker" {
			out.IsMaker = true
		} else {
			out.IsMaker = false
		}
		out.Symbol = trade.Market
		out.TimeStamp = trade.Time
		c.userTrade.insertTrade(out)
	}
	// clear
	c.fillDetect.before = nil
	c.fillDetect.after = nil
	return nil
}

func (c *Client) thisTradeIdWeHaveIt(tradeId float64) bool {
	for _, beTrade := range c.fillDetect.before.Result {
		if tradeId == beTrade.ID {
			return true
		}
	}
	return false
}
