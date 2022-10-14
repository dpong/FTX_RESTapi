package api

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const timeLayout = "2006-01-02T15:04:05.999999+00:00"

type OrderBookBranch struct {
	Bids        bookBranch
	Asks        bookBranch
	SnapShoted  bool
	Cancel      *context.CancelFunc
	reCh        chan error
	lastRefresh lastRefreshBranch
}

type lastRefreshBranch struct {
	mux  sync.RWMutex
	time time.Time
}

type bookBranch struct {
	mux  sync.RWMutex
	Book [][]string
}

func (o *OrderBookBranch) IfCanRefresh() bool {
	o.lastRefresh.mux.Lock()
	defer o.lastRefresh.mux.Unlock()
	now := time.Now()
	if now.After(o.lastRefresh.time.Add(time.Second * 3)) {
		o.lastRefresh.time = now
		return true
	}
	return false
}

func (o *OrderBookBranch) Close() {
	(*o.Cancel)()
	o.SnapShoted = true
	o.Bids.mux.Lock()
	o.Bids.Book = [][]string{}
	o.Bids.mux.Unlock()
	o.Asks.mux.Lock()
	o.Asks.Book = [][]string{}
	o.Asks.mux.Unlock()
}

// return bids, ready or not
func (o *OrderBookBranch) GetBids() ([][]string, bool) {
	o.Bids.mux.RLock()
	defer o.Bids.mux.RUnlock()
	if !o.SnapShoted {
		return [][]string{}, false
	}
	if len(o.Bids.Book) == 0 {
		if o.IfCanRefresh() {
			o.reCh <- errors.New("re cause len bid is zero")
		}
		return [][]string{}, false
	}
	book := o.Bids.Book
	return book, true
}

// return asks, ready or not
func (o *OrderBookBranch) GetAsks() ([][]string, bool) {
	o.Asks.mux.RLock()
	defer o.Asks.mux.RUnlock()
	if !o.SnapShoted {
		return [][]string{}, false
	}
	if len(o.Asks.Book) == 0 {
		if o.IfCanRefresh() {
			o.reCh <- errors.New("re cause len ask is zero")
		}
		return [][]string{}, false
	}
	book := o.Asks.Book
	return book, true
}

func reStartMainSeesionErrHub(err string) bool {
	switch {
	case strings.Contains(err, "reconnect because of time out"):
		return false
	case strings.Contains(err, "reconnect because of reCh send"):
		return false
	case strings.Contains(err, "reconnect because of ChannelOrderBook error"):
		return false
	}
	return true
}

func LocalOrderBook(symbol string, logger *log.Logger) *OrderBookBranch {
	var o OrderBookBranch
	ctx, cancel := context.WithCancel(context.Background())
	o.Cancel = &cancel
	bookticker := make(chan map[string]interface{}, 50)
	errCh := make(chan error, 1)
	refreshCh := make(chan error, 1)
	o.reCh = make(chan error, 5)
	symbol = strings.ToUpper(symbol)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := fTXOrderBookSocket(ctx, symbol, logger, &bookticker, &refreshCh); err == nil {
					return
				} else {
					if reStartMainSeesionErrHub(err.Error()) {
						errCh <- errors.New("reconnect websocket")
					}
					logger.Warningf("Reconnect %s websocket stream.\n", symbol)
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := o.maintainOrderBook(ctx, symbol, &bookticker, &errCh, &refreshCh)
				if err == nil {
					return
				}
				logger.Warningf("Refreshing %s local orderbook cause: %s\n", symbol, err.Error())
			}
		}
	}()
	return &o
}

func (o *OrderBookBranch) updateNewComing(message *map[string]interface{}) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// bid
		bids, ok := (*message)["bids"].([]interface{})
		if !ok {
			return
		}
		for _, bid := range bids {
			price := decimal.NewFromFloat(bid.([]interface{})[0].(float64))
			qty := decimal.NewFromFloat(bid.([]interface{})[1].(float64))
			o.dealWithBidPriceLevel(price, qty)
		}
	}()
	go func() {
		defer wg.Done()
		// ask
		asks, ok := (*message)["asks"].([]interface{})
		if !ok {
			return
		}
		for _, ask := range asks {
			price := decimal.NewFromFloat(ask.([]interface{})[0].(float64))
			qty := decimal.NewFromFloat(ask.([]interface{})[1].(float64))
			o.dealWithAskPriceLevel(price, qty)
		}
	}()
	wg.Wait()
}

func (o *OrderBookBranch) dealWithBidPriceLevel(price, qty decimal.Decimal) {
	o.Bids.mux.Lock()
	defer o.Bids.mux.Unlock()
	l := len(o.Bids.Book)
	for level, item := range o.Bids.Book {
		bookPrice, _ := decimal.NewFromString(item[0])
		switch {
		case price.GreaterThan(bookPrice):
			// insert level
			if qty.IsZero() {
				// ignore
				return
			}
			o.Bids.Book = append(o.Bids.Book, []string{})
			copy(o.Bids.Book[level+1:], o.Bids.Book[level:])
			fprice := price.InexactFloat64()
			fqty := qty.InexactFloat64()
			o.Bids.Book[level] = []string{floatHandle(fprice), floatHandle(fqty)}
			return
		case price.LessThan(bookPrice):
			if level == l-1 {
				// insert last level
				if qty.IsZero() {
					// ignore
					return
				}
				fprice := price.InexactFloat64()
				fqty := qty.InexactFloat64()
				o.Bids.Book = append(o.Bids.Book, []string{floatHandle(fprice), floatHandle(fqty)})
				return
			}
			continue
		case price.Equal(bookPrice):
			if qty.IsZero() {
				// delete level
				switch {
				case level == l-1:
					o.Bids.Book = o.Bids.Book[:l-1]
				default:
					o.Bids.Book = append(o.Bids.Book[:level], o.Bids.Book[level+1:]...)
				}
				return
			}
			fqty, _ := qty.Float64()
			o.Bids.Book[level][1] = floatHandle(fqty)
			return
		}
	}
}

func (o *OrderBookBranch) dealWithAskPriceLevel(price, qty decimal.Decimal) {
	o.Asks.mux.Lock()
	defer o.Asks.mux.Unlock()
	l := len(o.Asks.Book)
	for level, item := range o.Asks.Book {
		bookPrice, _ := decimal.NewFromString(item[0])
		switch {
		case price.LessThan(bookPrice):
			// insert level
			if qty.IsZero() {
				// ignore
				return
			}
			o.Asks.Book = append(o.Asks.Book, []string{})
			copy(o.Asks.Book[level+1:], o.Asks.Book[level:])
			fprice := price.InexactFloat64()
			fqty := qty.InexactFloat64()
			o.Asks.Book[level] = []string{floatHandle(fprice), floatHandle(fqty)}
			return
		case price.GreaterThan(bookPrice):
			if level == l-1 {
				// insert last level
				if qty.IsZero() {
					// ignore
					return
				}
				fprice := price.InexactFloat64()
				fqty := qty.InexactFloat64()
				o.Asks.Book = append(o.Asks.Book, []string{floatHandle(fprice), floatHandle(fqty)})
				return
			}
			continue
		case price.Equal(bookPrice):
			if qty.IsZero() {
				// delete level
				switch {
				case level == l-1:
					o.Asks.Book = o.Asks.Book[:l-1]
				default:
					o.Asks.Book = append(o.Asks.Book[:level], o.Asks.Book[level+1:]...)
				}
				return
			}
			fqty, _ := qty.Float64()
			o.Asks.Book[level][1] = floatHandle(fqty)
			return
		}
	}
}

func (o *OrderBookBranch) refreshLocalOrderBook(err error) error {
	if o.IfCanRefresh() {
		if len(o.reCh) == cap(o.reCh) {
			return errors.New("refresh channel is full, please check it up")
		}
		o.reCh <- err
	}
	return nil
}

func (o *OrderBookBranch) maintainOrderBook(
	ctx context.Context,
	symbol string,
	bookticker *chan map[string]interface{},
	errCh *chan error,
	reCh *chan error,
) error {
	o.SnapShoted = false
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-(*errCh):
			return err
		case err := <-o.reCh:
			errSend := errors.New("reconnect because of reCh send")
			(*reCh) <- errSend
			return err
		case message := <-(*bookticker):
			channel, ok := message["channel"].(string)
			if !ok {
				continue
			}
			switch channel {
			case "orderbook":
				if err := o.channelOrderBook(&message); err == nil {
				} else {
					errSend := errors.New("reconnect because of ChannelOrderBook error")
					(*reCh) <- errSend
					return err
				}
			}
		}
	}
}

func (o *OrderBookBranch) channelOrderBook(message *map[string]interface{}) error {
	data, ok := (*message)["data"].(map[string]interface{})
	if !ok {
		return errors.New("data is not ok")
	}
	if len(data) == 0 {
		return errors.New("data len is 0")
	}
	action, ok := data["action"].(string)
	if !ok {
		return errors.New("data action is not ok")
	}
	switch action {
	case "partial":
		o.initialOrderBook(&data)
	case "update":
		o.updateNewComing(&data)
		if checkSumf, ok := (*&data)["checksum"].(float64); !ok {
			return errors.New("did't get checksum after updateNewComing")
		} else {
			if err := o.checkCheckSum(uint32(checkSumf)); err != nil {
				return err
			}
		}
	}
	return nil
}

func string2Bytes(s string) []byte {
	return bytes.NewBufferString(s).Bytes()
}

func (o *OrderBookBranch) checkCheckSum(checkSum uint32) error {
	o.Bids.mux.RLock()
	o.Asks.mux.RLock()
	defer o.Bids.mux.RUnlock()
	defer o.Asks.mux.RUnlock()
	if len(o.Bids.Book) == 0 || len(o.Asks.Book) == 0 {
		return nil
	}
	bidLen := len(o.Bids.Book)
	askLen := len(o.Asks.Book)
	var list []string
	for i := 0; i < 100; i++ {
		if i < bidLen {
			list = append(list, o.Bids.Book[i][:2]...)
		}
		if i < askLen {
			list = append(list, o.Asks.Book[i][:2]...)
		}
	}
	result := strings.Join(list, ":")
	localCheckSum := crc32.ChecksumIEEE(string2Bytes(result))
	if localCheckSum != checkSum {
		return errors.New("checkSum error")
	}
	return nil
}

func floatHandle(f float64) (str string) {
	decX := decimal.NewFromFloat(f)
	switch {
	case f == float64(int(f)):
		str = strconv.FormatFloat(f, 'f', 1, 32)
	case decX.LessThan(decimal.NewFromFloat(0.0001)):
		str = strconv.FormatFloat(f, 'e', 1, 32)
	default:
		str = strconv.FormatFloat(f, 'f', -1, 32)
	}
	return str
}

func (o *OrderBookBranch) initialOrderBook(res *map[string]interface{}) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// bid
		o.Bids.mux.Lock()
		defer o.Bids.mux.Unlock()
		o.Bids.Book = [][]string{}
		bids := (*res)["bids"].([]interface{})
		for _, item := range bids {
			levelData := item.([]interface{})
			price := floatHandle(levelData[0].(float64))
			size := floatHandle(levelData[1].(float64))
			o.Bids.Book = append(o.Bids.Book, []string{price, size})
		}
	}()
	go func() {
		defer wg.Done()
		// ask
		o.Asks.mux.Lock()
		defer o.Asks.mux.Unlock()
		o.Asks.Book = [][]string{}
		asks := (*res)["asks"].([]interface{})
		for _, item := range asks {
			levelData := item.([]interface{})
			price := floatHandle(levelData[0].(float64))
			size := floatHandle(levelData[1].(float64))
			o.Asks.Book = append(o.Asks.Book, []string{price, size})
		}
	}()
	wg.Wait()
	o.SnapShoted = true
}

type fTXWebsocket struct {
	OnErr  bool
	Logger *log.Logger
	Conn   *websocket.Conn
}

type fTXSubscribeMessage struct {
	Op      string `json:"op"`
	Channel string `json:"channel,omitempty"`
	Market  string `json:"market,omitempty"`
}

type argsN struct {
	Key        string `json:"key"`
	Sign       string `json:"sign"`
	Time       int64  `json:"time"`
	Subaccount string `json:"subaccount"`
}

func fTXDecoding(message *[]byte) (res map[string]interface{}, err error) {
	if *message == nil {
		err = errors.New("the incoming message is nil")
		return nil, err
	}
	err = json.Unmarshal(*message, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func fTXOrderBookSocket(
	ctx context.Context,
	symbol string,
	logger *log.Logger,
	mainCh *chan map[string]interface{},
	reCh *chan error,
) error {
	var w fTXWebsocket
	var duration time.Duration = 30
	innerErr := make(chan error, 1)
	w.Logger = logger
	w.OnErr = false
	symbol = strings.ToUpper(symbol)
	url := "wss://ftx.com/ws/"
	// wait 5 second, if the hand shake fail, will terminate the dail
	dailCtx, _ := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	conn, _, err := websocket.DefaultDialer.DialContext(dailCtx, url, nil)
	if err != nil {
		return err
	}
	logger.Infof("FTX %s orderBook socket connected.\n", symbol)
	w.Conn = conn
	defer conn.Close()

	send, err := getFTXOrderBookSubscribeMessage(symbol)
	if err != nil {
		return err
	}
	if err := w.Conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	w.Conn.SetPingHandler(nil)
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
				if err := w.Conn.WriteMessage(websocket.TextMessage, send); err != nil {
					w.Conn.SetReadDeadline(time.Now().Add(time.Millisecond * 5))
					return
				}
				w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration))
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-(*reCh):
			innerErr <- errors.New("restart")
			return err
		default:
			_, buf, err := conn.ReadMessage()
			if err != nil {
				innerErr <- errors.New("restart")
				return err
			}
			res, err1 := fTXDecoding(&buf)
			if err1 != nil {
				innerErr <- errors.New("restart")
				return err1
			}
			err2 := handleFTXWebsocket(&res, mainCh)
			if err2 != nil {
				innerErr <- errors.New("restart")
				return err2
			}
			if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
				return err
			}
		}
	}
}

func handleFTXWebsocket(res *map[string]interface{}, mainCh *chan map[string]interface{}) error {
	switch (*res)["type"] {
	case "error":
		Msg := (*res)["msg"].(string)
		Code := (*res)["code"].(float64)
		sCode := strconv.FormatFloat(Code, 'f', -1, 64)
		var buffer bytes.Buffer
		buffer.WriteString("Code: ")
		buffer.WriteString(sCode)
		buffer.WriteString(" | ")
		buffer.WriteString(Msg)
		err := errors.New(buffer.String())
		return err
	case "subscribed":
		Channel := (*res)["channel"].(string)
		var buffer bytes.Buffer
		buffer.WriteString("Subscribed | Channel: ")
		buffer.WriteString(Channel)
		if Channel == "ticker" {
			Market := (*res)["market"].(string)
			buffer.WriteString(" | ")
			buffer.WriteString("Product: ")
			buffer.WriteString(Market)
		}
		log.Println(buffer.String())
	case "info":
		Code := (*res)["code"].(float64)
		if Code == 20001 {
			err := errors.New("server restarted, Code 20001")
			return err
		}
	case "partial":
		*mainCh <- *res
	case "update":
		Channel := (*res)["channel"].(string)
		switch Channel {
		case "ticker":
			if data, ok := (*res)["data"].(map[string]interface{}); ok {
				*mainCh <- data
			}
		default:
			*mainCh <- *res
		}
	default:
		//pass
	}
	return nil
}

func getFTXOrderBookSubscribeMessage(market string) ([]byte, error) {
	sub := fTXSubscribeMessage{Op: "subscribe", Channel: "orderbook", Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}

func getFTXTradesSubscribeMessage(market string) ([]byte, error) {
	sub := fTXSubscribeMessage{Op: "subscribe", Channel: "trades", Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}
