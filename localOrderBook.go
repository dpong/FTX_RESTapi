package api

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const timeLayout = "2006-01-02T15:04:05.999999+00:00"

type OrderBookBranch struct {
	Bids        BookBranch
	Asks        BookBranch
	SnapShoted  bool
	Cancel      *context.CancelFunc
	BuyTrade    TradeImpact
	SellTrade   TradeImpact
	LookBack    time.Duration
	fromLevel   int
	toLevel     int
	reCh        chan error
	lastRefresh lastRefreshBranch
}

type lastRefreshBranch struct {
	mux  sync.RWMutex
	time time.Time
}

type TradeImpact struct {
	mux      sync.RWMutex
	Stamp    []time.Time
	Qty      []decimal.Decimal
	Notional []decimal.Decimal
}

type BookBranch struct {
	mux   sync.RWMutex
	Book  [][]string
	Micro []BookMicro
}

type BookMicro struct {
	OrderNum int
	Trend    string
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

func (o *OrderBookBranch) UpdateNewComing(message *map[string]interface{}) {
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
			o.DealWithBidPriceLevel(price, qty)
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
			o.DealWithAskPriceLevel(price, qty)
		}
	}()
	wg.Wait()
}

func (o *OrderBookBranch) DealWithBidPriceLevel(price, qty decimal.Decimal) {
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
			// micro part
			o.Bids.Micro = append(o.Bids.Micro, BookMicro{})
			copy(o.Bids.Micro[level+1:], o.Bids.Micro[level:])
			fprice, _ := price.Float64()
			fqty, _ := qty.Float64()
			o.Bids.Book[level] = []string{FloatHandle(fprice), FloatHandle(fqty)}
			o.Bids.Micro[level].OrderNum = 1
			return
		case price.LessThan(bookPrice):
			if level == l-1 {
				// insert last level
				if qty.IsZero() {
					// ignore
					return
				}
				fprice, _ := price.Float64()
				fqty, _ := qty.Float64()
				o.Bids.Book = append(o.Bids.Book, []string{FloatHandle(fprice), FloatHandle(fqty)})
				data := BookMicro{
					OrderNum: 1,
				}
				o.Bids.Micro = append(o.Bids.Micro, data)
				return
			}
			continue
		case price.Equal(bookPrice):
			if qty.IsZero() {
				// delete level
				switch {
				case level == l-1:
					o.Bids.Book = o.Bids.Book[:l-1]
					o.Bids.Micro = o.Bids.Micro[:l-1]
				default:
					o.Bids.Book = append(o.Bids.Book[:level], o.Bids.Book[level+1:]...)
					o.Bids.Micro = append(o.Bids.Micro[:level], o.Bids.Micro[level+1:]...)
				}
				return
			}
			fqty, _ := qty.Float64()
			oldQty, _ := decimal.NewFromString(o.Bids.Book[level][1])
			switch {
			case oldQty.GreaterThan(qty):
				// add order
				o.Bids.Micro[level].OrderNum++
				o.Bids.Micro[level].Trend = "add"
			case oldQty.LessThan(qty):
				// cut order
				o.Bids.Micro[level].OrderNum--
				o.Bids.Micro[level].Trend = "cut"
				if o.Bids.Micro[level].OrderNum < 1 {
					o.Bids.Micro[level].OrderNum = 1
				}
			}
			o.Bids.Book[level][1] = FloatHandle(fqty)
			return
		}
	}
}

func (o *OrderBookBranch) DealWithAskPriceLevel(price, qty decimal.Decimal) {
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
			// micro part
			o.Asks.Micro = append(o.Asks.Micro, BookMicro{})
			copy(o.Asks.Micro[level+1:], o.Asks.Micro[level:])
			fprice, _ := price.Float64()
			fqty, _ := qty.Float64()
			o.Asks.Book[level] = []string{FloatHandle(fprice), FloatHandle(fqty)}
			o.Asks.Micro[level].OrderNum = 1
			return
		case price.GreaterThan(bookPrice):
			if level == l-1 {
				// insert last level
				if qty.IsZero() {
					// ignore
					return
				}
				fprice, _ := price.Float64()
				fqty, _ := qty.Float64()
				o.Asks.Book = append(o.Asks.Book, []string{FloatHandle(fprice), FloatHandle(fqty)})
				data := BookMicro{
					OrderNum: 1,
				}
				o.Asks.Micro = append(o.Asks.Micro, data)
				return
			}
			continue
		case price.Equal(bookPrice):
			if qty.IsZero() {
				// delete level
				switch {
				case level == l-1:
					o.Asks.Book = o.Asks.Book[:l-1]
					o.Asks.Micro = o.Asks.Micro[:l-1]
				default:
					o.Asks.Book = append(o.Asks.Book[:level], o.Asks.Book[level+1:]...)
					o.Asks.Micro = append(o.Asks.Micro[:level], o.Asks.Micro[level+1:]...)
				}
				return
			}
			fqty, _ := qty.Float64()
			oldQty, _ := decimal.NewFromString(o.Asks.Book[level][1])
			switch {
			case oldQty.GreaterThan(qty):
				// add order
				o.Asks.Micro[level].OrderNum++
				o.Asks.Micro[level].Trend = "add"
			case oldQty.LessThan(qty):
				// cut order
				o.Asks.Micro[level].OrderNum--
				o.Asks.Micro[level].Trend = "cut"
				if o.Asks.Micro[level].OrderNum < 1 {
					o.Asks.Micro[level].OrderNum = 1
				}
			}
			o.Asks.Book[level][1] = FloatHandle(fqty)
			return
		}
	}
}

func (o *OrderBookBranch) RefreshLocalOrderBook(err error) {
	if o.IfCanRefresh() {
		o.reCh <- err
	}
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

func (o *OrderBookBranch) SetLookBackSec(input int) {
	o.LookBack = time.Duration(input) * time.Second
}

// top of the book is 1, to the level you want to sum all the notions
func (o *OrderBookBranch) SetImpactCumRange(toLevel int) {
	o.fromLevel = 0
	o.toLevel = toLevel - 1
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

func (o *OrderBookBranch) GetBidMicro(idx int) (*BookMicro, bool) {
	o.Bids.mux.RLock()
	defer o.Bids.mux.RUnlock()
	if len(o.Bids.Book) == 0 || !o.SnapShoted {
		return nil, false
	}
	micro := o.Bids.Micro[idx]
	return &micro, true
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

func (o *OrderBookBranch) GetAskMicro(idx int) (*BookMicro, bool) {
	o.Asks.mux.RLock()
	defer o.Asks.mux.RUnlock()
	if len(o.Asks.Book) == 0 || !o.SnapShoted {
		return nil, false
	}
	micro := o.Asks.Micro[idx]
	return &micro, true
}

func (o *OrderBookBranch) GetBuyImpactNotion() decimal.Decimal {
	o.BuyTrade.mux.RLock()
	defer o.BuyTrade.mux.RUnlock()
	var total decimal.Decimal
	now := time.Now()
	for i, st := range o.BuyTrade.Stamp {
		if now.After(st.Add(o.LookBack)) {
			continue
		}
		total = total.Add(o.BuyTrade.Notional[i])
	}
	return total
}

func (o *OrderBookBranch) GetSellImpactNotion() decimal.Decimal {
	o.SellTrade.mux.RLock()
	defer o.SellTrade.mux.RUnlock()
	var total decimal.Decimal
	now := time.Now()
	for i, st := range o.SellTrade.Stamp {
		if now.After(st.Add(o.LookBack)) {
			continue
		}
		total = total.Add(o.SellTrade.Notional[i])
	}
	return total
}

func (o *OrderBookBranch) CalBidCumNotional() (decimal.Decimal, bool) {
	if len(o.Bids.Book) == 0 {
		return decimal.NewFromFloat(0), false
	}
	if o.fromLevel > o.toLevel {
		return decimal.NewFromFloat(0), false
	}
	o.Bids.mux.RLock()
	defer o.Bids.mux.RUnlock()
	var total decimal.Decimal
	for level, item := range o.Bids.Book {
		if level >= o.fromLevel && level <= o.toLevel {
			price, _ := decimal.NewFromString(item[0])
			qty, _ := decimal.NewFromString(item[1])
			total = total.Add(qty.Mul(price))
		} else if level > o.toLevel {
			break
		}
	}
	return total, true
}

func (o *OrderBookBranch) CalAskCumNotional() (decimal.Decimal, bool) {
	if len(o.Asks.Book) == 0 {
		return decimal.NewFromFloat(0), false
	}
	if o.fromLevel > o.toLevel {
		return decimal.NewFromFloat(0), false
	}
	o.Asks.mux.RLock()
	defer o.Asks.mux.RUnlock()
	var total decimal.Decimal
	for level, item := range o.Asks.Book {
		if level >= o.fromLevel && level <= o.toLevel {
			price, _ := decimal.NewFromString(item[0])
			qty, _ := decimal.NewFromString(item[1])
			total = total.Add(qty.Mul(price))
		} else if level > o.toLevel {
			break
		}
	}
	return total, true
}

func (o *OrderBookBranch) IsBigImpactOnBid() bool {
	impact := o.GetSellImpactNotion()
	rest, ok := o.CalBidCumNotional()
	if !ok {
		return false
	}
	micro, ok := o.GetBidMicro(o.fromLevel)
	if !ok {
		return false
	}
	if impact.GreaterThanOrEqual(rest) && micro.Trend == "cut" {
		return true
	}
	return false
}

func (o *OrderBookBranch) IsBigImpactOnAsk() bool {
	impact := o.GetBuyImpactNotion()
	rest, ok := o.CalAskCumNotional()
	if !ok {
		return false
	}
	micro, ok := o.GetAskMicro(o.fromLevel)
	if !ok {
		return false
	}
	if impact.GreaterThanOrEqual(rest) && micro.Trend == "cut" {
		return true
	}
	return false
}

func ReStartMainSeesionErrHub(err string) bool {
	switch {
	case strings.Contains(err, "reconnect because of time out"):
		return false
	case strings.Contains(err, "reconnect because of reCh send"):
		return false
	}
	return true
}

func LocalOrderBook(symbol string, logger *log.Logger, streamTrade bool) *OrderBookBranch {
	var o OrderBookBranch
	o.SetLookBackSec(5) // default 5 sec
	o.SetImpactCumRange(20)
	ctx, cancel := context.WithCancel(context.Background())
	o.Cancel = &cancel
	bookticker := make(chan map[string]interface{}, 50)
	errCh := make(chan error, 1)
	refreshCh := make(chan string, 1)
	orderBookErr := make(chan error, 1)
	o.reCh = make(chan error, 5)
	symbol = strings.ToUpper(symbol)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := FTXOrderBookSocket(ctx, symbol, logger, &bookticker, &errCh, &refreshCh, &orderBookErr, streamTrade); err == nil {
					return
				} else {
					if ReStartMainSeesionErrHub(err.Error()) {
						errCh <- errors.New("Reconnect websocket")
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
				err := o.MaintainOrderBook(ctx, symbol, &bookticker, &errCh, &refreshCh, &orderBookErr)
				if err == nil {
					return
				}
				logger.Warningf("Refreshing %s local orderbook cause: %s\n", symbol, err.Error())
			}
		}
	}()
	return &o
}

func (o *OrderBookBranch) MaintainOrderBook(
	ctx context.Context,
	symbol string,
	bookticker *chan map[string]interface{},
	errCh *chan error,
	refreshCh *chan string,
	reCh *chan error,
) error {
	o.SnapShoted = false
	lastUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-(*errCh):
			return err
		case err := <-o.reCh:
			errSend := errors.New("reconnect because of time out")
			(*reCh) <- errSend
			return err
		case message := <-(*bookticker):
			channel, ok := message["channel"].(string)
			if !ok {
				continue
			}
			switch channel {
			case "orderbook":
				if err := o.ChannelOrderBook(&message, refreshCh); err == nil {
					lastUpdate = time.Now()
				}
			case "trades":
				o.ChannelTrades(&message)
			}
		default:
			if time.Now().After(lastUpdate.Add(time.Second * 10)) {
				// 10 sec without updating
				err := errors.New("reconnect because of time out")
				(*reCh) <- err
				return err
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func (o *OrderBookBranch) ChannelOrderBook(message *map[string]interface{}, refreshCh *chan string) error {
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
		o.InitialOrderBook(&data)
	case "update":
		o.UpdateNewComing(&data)
		checkSum := uint32((*&data)["checksum"].(float64))
		if err := o.CheckCheckSum(checkSum); err != nil {
			// restart local orderbook
			*refreshCh <- "refresh"
			return err
		}
	}
	return nil
}

func (o *OrderBookBranch) ChannelTrades(message *map[string]interface{}) {
	data, ok := (*message)["data"].([]interface{})
	if !ok {
		return
	}
	if len(data) == 0 {
		return
	}
	for _, item := range data {
		itemMap := item.(map[string]interface{})
		price := decimal.NewFromFloat(itemMap["price"].(float64))
		size := decimal.NewFromFloat(itemMap["size"].(float64))
		side := itemMap["side"].(string)
		stamp, err := time.Parse(timeLayout, itemMap["time"].(string))
		if err != nil {
			continue
		}
		o.LocateTradeImpact(side, price, size, stamp)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	now := time.Now()
	go func() {
		defer wg.Done()
		o.BuyTrade.mux.Lock()
		defer o.BuyTrade.mux.Unlock()
		var loc int = -1
		for i, st := range o.BuyTrade.Stamp {
			if !now.After(st.Add(o.LookBack)) {
				break
			}
			loc = i
		}
		if loc == -1 {
			return
		}
		o.BuyTrade.Stamp = o.BuyTrade.Stamp[loc+1:]
		o.BuyTrade.Qty = o.BuyTrade.Qty[loc+1:]
		o.BuyTrade.Notional = o.BuyTrade.Notional[loc+1:]
	}()
	go func() {
		defer wg.Done()
		o.SellTrade.mux.Lock()
		defer o.SellTrade.mux.Unlock()
		var loc int = -1
		for i, st := range o.SellTrade.Stamp {
			if !now.After(st.Add(o.LookBack)) {
				break
			}
			loc = i
		}
		if loc == -1 {
			return
		}
		o.SellTrade.Stamp = o.SellTrade.Stamp[loc+1:]
		o.SellTrade.Qty = o.SellTrade.Qty[loc+1:]
		o.SellTrade.Notional = o.SellTrade.Notional[loc+1:]
	}()
	wg.Wait()
}

func String2Bytes(s string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh := reflect.SliceHeader{
		Data: sh.Data,
		Len:  sh.Len,
		Cap:  sh.Len,
	}
	return *(*[]byte)(unsafe.Pointer(&bh))
}

func (o *OrderBookBranch) CheckCheckSum(checkSum uint32) error {
	o.Bids.mux.RLock()
	o.Asks.mux.RLock()
	defer o.Bids.mux.RUnlock()
	defer o.Asks.mux.RUnlock()
	if len(o.Bids.Book) == 0 || len(o.Asks.Book) == 0 {
		return nil
	}
	var list []string
	for i := 0; i < 100; i++ {
		list = append(list, o.Bids.Book[i][:2]...)
		list = append(list, o.Asks.Book[i][:2]...)
	}
	result := strings.Join(list, ":")
	localCheckSum := crc32.ChecksumIEEE(String2Bytes(result))
	if localCheckSum != checkSum {
		return errors.New("checkSum error")
	}
	return nil
}

func (o *OrderBookBranch) LocateTradeImpact(side string, price, size decimal.Decimal, st time.Time) {
	switch side {
	case "buy":
		o.BuyTrade.mux.Lock()
		defer o.BuyTrade.mux.Unlock()
		o.BuyTrade.Qty = append(o.BuyTrade.Qty, size)
		o.BuyTrade.Stamp = append(o.BuyTrade.Stamp, st)
		o.BuyTrade.Notional = append(o.BuyTrade.Notional, price.Mul(size))
	case "sell":
		o.SellTrade.mux.Lock()
		defer o.SellTrade.mux.Unlock()
		o.SellTrade.Qty = append(o.SellTrade.Qty, size)
		o.SellTrade.Stamp = append(o.SellTrade.Stamp, st)
		o.SellTrade.Notional = append(o.SellTrade.Notional, price.Mul(size))
	}
}

func FloatHandle(f float64) string {
	if float64(f) == float64(int(f)) {
		return strconv.FormatFloat(float64(f), 'f', 1, 32)
	}
	return strconv.FormatFloat(float64(f), 'f', -1, 32)
}

func (o *OrderBookBranch) InitialOrderBook(res *map[string]interface{}) {
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
			price := FloatHandle(levelData[0].(float64))
			size := FloatHandle(levelData[1].(float64))
			o.Bids.Book = append(o.Bids.Book, []string{price, size})
			// micro part
			micro := BookMicro{
				OrderNum: 1, // initial order num is 1
			}
			o.Bids.Micro = append(o.Bids.Micro, micro)
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
			price := FloatHandle(levelData[0].(float64))
			size := FloatHandle(levelData[1].(float64))
			o.Asks.Book = append(o.Asks.Book, []string{price, size})
			// micro part
			micro := BookMicro{
				OrderNum: 1, // initial order num is 1
			}
			o.Asks.Micro = append(o.Asks.Micro, micro)
		}
	}()
	wg.Wait()
	o.SnapShoted = true
}

type FTXWesocket struct {
	OnErr  bool
	Logger *log.Logger
	Conn   *websocket.Conn
}

type FTXSubscribeMessage struct {
	Op      string `json:"op"`
	Channel string `json:"channel,omitempty"`
	Market  string `json:"market,omitempty"`
}

type ArgsN struct {
	Key        string `json:"key"`
	Sign       string `json:"sign"`
	Time       int64  `json:"time"`
	Subaccount string `json:"subaccount"`
}

func (w *FTXWesocket) OutFTXErr() map[string]interface{} {
	w.OnErr = true
	m := make(map[string]interface{})
	return m
}

func FTXDecoding(message *[]byte) (res map[string]interface{}, err error) {
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

func FTXOrderBookSocket(
	ctx context.Context,
	symbol string,
	logger *log.Logger,
	mainCh *chan map[string]interface{},
	errCh *chan error,
	refreshCh *chan string,
	reCh *chan error,
	streamTrade bool,
) error {
	var w FTXWesocket
	var duration time.Duration = 300
	w.Logger = logger
	w.OnErr = false
	symbol = strings.ToUpper(symbol)
	url := "wss://ftx.com/ws/"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	logger.Infof("FTX %s orderBook socket connected.\n", symbol)
	w.Conn = conn
	defer conn.Close()

	send, err := GetFTXOrderBookSubscribeMessage(symbol)
	if err != nil {
		return err
	}
	if err := w.Conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	if streamTrade {
		send, err = GetFTXTradesSubscribeMessage(symbol)
		if err != nil {
			return err
		}
		if err := w.Conn.WriteMessage(websocket.TextMessage, send); err != nil {
			return err
		}
	}
	if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	read := time.NewTicker(time.Millisecond * 50)
	defer read.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-*refreshCh:
			return errors.New("refresh")
		case err := <-(*reCh):
			return err
		case <-read.C:
			if conn == nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return errors.New(message)
			}
			_, buf, err := conn.ReadMessage()
			if err != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return errors.New(message)
			}
			res, err1 := FTXDecoding(&buf)
			if err1 != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message, err1)
				return err1
			}

			err2 := HandleFTXWebsocket(&res, mainCh)
			if err2 != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return err2
			}
			if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
				return err
			}
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func HandleFTXWebsocket(res *map[string]interface{}, mainCh *chan map[string]interface{}) error {
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
			err := errors.New("Server Restarted，Code 20001。")
			return err
		}
	case "partial":
		*mainCh <- *res
	case "update":
		*mainCh <- *res
	default:
		//pass
	}
	return nil
}

func GetFTXOrderBookSubscribeMessage(market string) ([]byte, error) {
	sub := FTXSubscribeMessage{Op: "subscribe", Channel: "orderbook", Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}

func GetFTXTradesSubscribeMessage(market string) ([]byte, error) {
	sub := FTXSubscribeMessage{Op: "subscribe", Channel: "trades", Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}
