package bitfinex

// Copyright (c) 2015-2019 Bitontop Technologies Inc.
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/bitontop/gored/coin"
	"github.com/bitontop/gored/exchange"
	"github.com/bitontop/gored/pair"
)

/*The Base Endpoint URL*/
const (
	API_URL = "https://api.bitfinex.com"
)

/*API Base Knowledge
Path: API function. Usually after the base endpoint URL
Method:
	Get - Call a URL, API return a response
	Post - Call a URL & send a request, API return a response
Public API:
	It doesn't need authorization/signature , can be called by browser to get response.
	using exchange.HttpGetRequest/exchange.HttpPostRequest
Private API:
	Authorization/Signature is requried. The signature request should look at Exchange API Document.
	using ApiKeyGet/ApiKeyPost
Response:
	Response is a json structure.
	Copy the json to https://transform.now.sh/json-to-go/ convert to go Struct.
	Add the go Struct to model.go

ex. Get /api/v1/depth
Get - Method
/api/v1/depth - Path*/

/*************** Public API ***************/
/*Get Coins Information (If API provide)
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Modify API Path(strRequestUrl)*/
func (e *Bitfinex) GetCoinsData() {
	coinsData := CoinsData{}

	fields := []string{"pub:map:currency:sym", "pub:map:currency:label"}
	strRequestUrl := "/v2/conf/"

	for i, field := range fields {
		strURL := API_URL + strRequestUrl + field

		jsonCurrencyReturn := exchange.HttpGetRequest(strURL, nil)
		if err := json.Unmarshal([]byte(jsonCurrencyReturn), &coinsData); err != nil {
			log.Printf("%s Get Coins Data Unmarshal Err: %v %v", e.GetName(), err, jsonCurrencyReturn)
			return
		}

		switch i {
		case 0:
			for _, fixSymbol := range coinsData[0] {
				c := &coin.Coin{}
				switch e.Source {
				case exchange.EXCHANGE_API:
					c = coin.GetCoin(fixSymbol[1])
					if c == nil {
						c = &coin.Coin{}
						c.Code = fixSymbol[1]
						coin.AddCoin(c)
					}
				case exchange.JSON_FILE:
					c = e.GetCoinBySymbol(fixSymbol[1])
				}
				if c != nil {
					coinConstraint := &exchange.CoinConstraint{
						CoinID:       c.ID,
						Coin:         c,
						ExSymbol:     fixSymbol[0],
						TxFee:        DEFAULT_TXFEE,
						Withdraw:     DEFAULT_WITHDRAW,
						Deposit:      DEFAULT_DEPOSIT,
						Confirmation: DEFAULT_CONFIRMATION,
						Listed:       true,
					}
					e.SetCoinConstraint(coinConstraint)
				}
			}
		case 1:
			for _, symbol := range coinsData[0] {
				c := e.GetCoinBySymbol(symbol[0])
				switch e.Source {
				case exchange.EXCHANGE_API:
					if c == nil {
						c = coin.GetCoin(symbol[0])
						if c == nil {
							c = &coin.Coin{}
							c.Code = symbol[0]
							c.Name = symbol[1]
							coin.AddCoin(c)
						}
					}
				case exchange.JSON_FILE:
					c = e.GetCoinBySymbol(symbol[0])
				}

				if c != nil {
					coinConstraint := &exchange.CoinConstraint{
						CoinID:       c.ID,
						Coin:         c,
						ExSymbol:     symbol[0],
						TxFee:        DEFAULT_TXFEE,
						Withdraw:     DEFAULT_WITHDRAW,
						Deposit:      DEFAULT_DEPOSIT,
						Confirmation: DEFAULT_CONFIRMATION,
						Listed:       true,
					}
					e.SetCoinConstraint(coinConstraint)
				}
			}
		}
	}

	e.GetWithdrawFees()
}

func (e *Bitfinex) GetWithdrawFees() {
	if e.API_KEY == "" || e.API_SECRET == "" {
		log.Printf("%s API Key or Secret Key are nil.", e.GetName())
		return
	}

	withdrawFee := WithdrawFee{}
	strRequestUrl := "/v1/account_fees"

	jsonFeesReturn := e.ApiKeyPost(make(map[string]interface{}), strRequestUrl)
	if err := json.Unmarshal([]byte(jsonFeesReturn), &withdrawFee); err != nil {
		log.Printf("%s GetWithdrawFees Data Unmarshal Err: %v %v", e.GetName(), err, jsonFeesReturn)
		return
	}

	for symbol, fee := range withdrawFee.Withdraw {

		c := e.GetCoinBySymbol(symbol)
		if c != nil {
			coinConstraint := e.GetCoinConstraint(c)
			coinConstraint.TxFee, _ = strconv.ParseFloat(fmt.Sprintf("%v", fee), 64)
		}
	}
}

/* GetPairsData - Get Pairs Information (If API provide)
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Modify API Path(strRequestUrl)*/
func (e *Bitfinex) GetPairsData() {
	pairsData := PairsData{}

	strRequestUrl := "/v1/symbols_details"
	strUrl := API_URL + strRequestUrl

	jsonSymbolsReturn := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonSymbolsReturn), &pairsData); err != nil {
		log.Printf("%s Get Pairs Data Unmarshal Err: %v %v", e.GetName(), err, jsonSymbolsReturn)
		return
	}

	baseList := []string{"usd", "eur", "gbp", "jpy", "btc", "eth", "eos", "xlm", "dai", "ust"}
	for _, data := range pairsData {
		for _, baseSymbol := range baseList {
			p := &pair.Pair{}
			strReg := fmt.Sprintf(`([^ ].+?)%s`, baseSymbol)
			reg := regexp.MustCompile(strReg)
			pairSymbol := reg.FindString(data.Pair)
			if pairSymbol != "" {
				targetSymbol := reg.ReplaceAllString(data.Pair, "$1")
				switch e.Source {
				case exchange.EXCHANGE_API:
					base := coin.GetCoin(baseSymbol)
					target := coin.GetCoin(targetSymbol)
					if base != nil && target != nil {
						p = pair.GetPair(base, target)
					}
				case exchange.JSON_FILE:
					p = e.GetPairBySymbol(data.Pair)
				}

				if p != nil {
					pairConstraint := &exchange.PairConstraint{
						PairID:      p.ID,
						Pair:        p,
						ExSymbol:    data.Pair,
						MakerFee:    DEFAULT_MAKER_FEE,
						TakerFee:    DEFAULT_TAKER_FEE,
						LotSize:     DEFAULT_LOT_SIZE,
						PriceFilter: math.Pow10(data.PricePrecision * -1),
						Listed:      true,
					}
					e.SetPairConstraint(pairConstraint)
				}
				break
			}
		}
	}
}

/*Get Pair Market Depth
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Get Exchange Pair Code ex. symbol := e.GetPairCode(p)
Step 4: Modify API Path(strRequestUrl)
Step 5: Add Params - Depend on API request
Step 6: Convert the response to Standard Maker struct*/
func (e *Bitfinex) OrderBook(p *pair.Pair) (*exchange.Maker, error) {
	orderBook := OrderBook{}
	symbol := e.GetSymbolByPair(p)

	strRequestUrl := fmt.Sprintf("/v1/book/%s", symbol)
	strUrl := API_URL + strRequestUrl

	maker := &exchange.Maker{}
	maker.WorkerIP = exchange.GetExternalIP()
	maker.BeforeTimestamp = float64(time.Now().UnixNano() / 1e6)

	jsonOrderbook := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonOrderbook), &orderBook); err != nil {
		return nil, fmt.Errorf("%s OrderBook json Unmarshal error: %v %v", e.GetName(), err, jsonOrderbook)
	}

	maker.AfterTimestamp = float64(time.Now().UnixNano() / 1e6)
	var err error
	for _, bid := range orderBook.Bids {
		buydata := exchange.Order{}
		buydata.Quantity, err = strconv.ParseFloat(bid.Amount, 64)
		if err != nil {
			return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
		}

		buydata.Rate, err = strconv.ParseFloat(bid.Price, 64)
		if err != nil {
			return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
		}
		maker.Bids = append(maker.Bids, buydata)
	}

	for _, ask := range orderBook.Asks {
		selldata := exchange.Order{}
		selldata.Quantity, err = strconv.ParseFloat(ask.Amount, 64)
		if err != nil {
			return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
		}

		selldata.Rate, err = strconv.ParseFloat(ask.Price, 64)
		if err != nil {
			return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
		}
		maker.Asks = append(maker.Asks, selldata)
	}

	return maker, err
}

/*************** Private API ***************/
func (e *Bitfinex) UpdateAllBalances() {
	if e.API_KEY == "" || e.API_SECRET == "" {
		log.Printf("%s API Key or Secret Key are nil.", e.GetName())
		return
	}

	accountBalance := AccountBalances{}
	strRequest := "/v1/balances"

	jsonBalanceReturn := e.ApiKeyPost(make(map[string]interface{}), strRequest)
	if err := json.Unmarshal([]byte(jsonBalanceReturn), &accountBalance); err != nil {
		log.Printf("%s UpdateAllBalances json Unmarshal error: %v %s", e.GetName(), err, jsonBalanceReturn)
		return
	} else {
		for _, balance := range accountBalance {
			freeamount, err := strconv.ParseFloat(balance.Available, 64)
			if err != nil {
				log.Printf("%s UpdateAllBalances err: %+v %v", e.GetName(), balance, err)
				return
			} else {
				c := e.GetCoinBySymbol(balance.Currency)
				if c != nil {
					balanceMap.Set(c.Code, freeamount)
				}
			}
		}
	}
}

/* Withdraw(coin *coin.Coin, quantity float64, addr, tag string) */
func (e *Bitfinex) Withdraw(coin *coin.Coin, quantity float64, addr, tag string) bool {
	return false
}

func (e *Bitfinex) LimitSell(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return nil, fmt.Errorf("%s API Key or Secret Key are nil.", e.GetName())
	}

	placeOrder := PlaceOrder{}
	strRequest := "/v1/order/new"

	mapParams := make(map[string]interface{})
	mapParams["symbol"] = e.GetSymbolByPair(pair)
	mapParams["amount"] = fmt.Sprintf("%v", quantity)
	mapParams["price"] = fmt.Sprintf("%v", rate)
	mapParams["side"] = "sell"
	mapParams["type"] = "exchange limit"

	jsonPlaceReturn := e.ApiKeyPost(mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonPlaceReturn), &placeOrder); err != nil {
		return nil, fmt.Errorf("%s LimitSell Unmarshal Err: %v %v", e.GetName(), err, jsonPlaceReturn)
	}

	order := &exchange.Order{
		Pair:         pair,
		OrderID:      fmt.Sprintf("%d", placeOrder.OrderID),
		Rate:         rate,
		Quantity:     quantity,
		Side:         "Sell",
		Status:       exchange.New,
		JsonResponse: jsonPlaceReturn,
	}
	return order, nil
}

func (e *Bitfinex) LimitBuy(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return nil, fmt.Errorf("%s API Key or Secret Key are nil.", e.GetName())
	}

	placeOrder := PlaceOrder{}
	strRequest := "/v1/order/new"

	mapParams := make(map[string]interface{})
	mapParams["symbol"] = e.GetSymbolByPair(pair)
	mapParams["amount"] = fmt.Sprintf("%v", quantity)
	mapParams["price"] = fmt.Sprintf("%v", rate)
	mapParams["side"] = "buy"
	mapParams["type"] = "exchange limit"

	jsonPlaceReturn := e.ApiKeyPost(mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonPlaceReturn), &placeOrder); err != nil {
		return nil, fmt.Errorf("%s LimitSell Unmarshal Err: %v %v", e.GetName(), err, jsonPlaceReturn)
	}

	order := &exchange.Order{
		Pair:         pair,
		OrderID:      fmt.Sprintf("%d", placeOrder.OrderID),
		Rate:         rate,
		Quantity:     quantity,
		Side:         "Buy",
		Status:       exchange.New,
		JsonResponse: jsonPlaceReturn,
	}

	return order, nil
}

func (e *Bitfinex) OrderStatus(order *exchange.Order) error {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return fmt.Errorf("%s API Key or Secret Key are nil.", e.GetName())
	}

	orderStatus := PlaceOrder{}
	strRequest := "/v1/order/status"

	mapParams := make(map[string]interface{})
	mapParams["order_id"], _ = strconv.Atoi(order.OrderID)

	jsonOrderStatus := e.ApiKeyPost(mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonOrderStatus), &orderStatus); err != nil {
		return fmt.Errorf("%s OrderStatus Unmarshal Err: %v %v", e.GetName(), err, jsonOrderStatus)
	} else if orderStatus.OrderID == 0 {
		return fmt.Errorf("%s Get OrderStatus Failed: %s", e.GetName(), jsonOrderStatus)
	}

	if orderStatus.IsLive {
		remain, _ := strconv.ParseFloat(orderStatus.RemainingAmount, 64)

		if remain == 0 {
			order.Status = exchange.Filled
		} else if remain > 0 && remain != order.Quantity {
			order.Status = exchange.Partial
		} else {
			order.Status = exchange.New
		}
	} else if orderStatus.IsCancelled {
		order.Status = exchange.Canceled
	} else {
		order.Status = exchange.Other
	}

	order.DealRate = order.Rate
	order.DealQuantity, _ = strconv.ParseFloat(orderStatus.ExecutedAmount, 64)

	return nil
}

func (e *Bitfinex) ListOrders() ([]*exchange.Order, error) {
	return nil, nil
}

func (e *Bitfinex) CancelOrder(order *exchange.Order) error {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return fmt.Errorf("%s API Key or Secret Key are nil.", e.GetName())
	}

	cancelOrder := PlaceOrder{}
	strRequest := "/v1/order/cancel"

	mapParams := make(map[string]interface{})
	mapParams["order_id"], _ = strconv.Atoi(order.OrderID)

	jsonCancelOrder := e.ApiKeyPost(mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonCancelOrder), &cancelOrder); err != nil {
		return fmt.Errorf("%s CancelOrder Unmarshal Err: %v %v", e.GetName(), err, jsonCancelOrder)
	} else if cancelOrder.OrderID == 0 {
		return fmt.Errorf("%s CancelOrder Failed: %s", e.GetName(), jsonCancelOrder)
	}

	order.Status = exchange.Canceling
	order.CancelStatus = jsonCancelOrder

	return nil
}

func (e *Bitfinex) CancelAllOrder() error {
	return nil
}

/*************** Signature Http Request ***************/
/*Method: API Request and Signature is required
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Create mapParams Depend on API Signature request
Step 3: Add HttpGetRequest below strUrl if API has different requests*/
func (e *Bitfinex) ApiKeyPost(mapParams map[string]interface{}, strRequestPath string) string {
	strMethod := "POST"

	mapParams["request"] = strRequestPath
	mapParams["nonce"] = fmt.Sprintf("%v", time.Now().UnixNano())

	//Signature Request Params
	payload, _ := json.Marshal(mapParams)
	payload_enc := base64.StdEncoding.EncodeToString(payload)
	Signature := ComputeHmac512_384NoDecode(payload_enc, e.API_SECRET)

	strUrl := API_URL + strRequestPath

	httpClient := &http.Client{}

	request, err := http.NewRequest(strMethod, strUrl, nil)
	if nil != err {
		return err.Error()
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")
	request.Header.Add("X-BFX-APIKEY", e.API_KEY)
	request.Header.Add("X-BFX-PAYLOAD", payload_enc)
	request.Header.Add("X-BFX-SIGNATURE", Signature)

	response, err := httpClient.Do(request)
	if nil != err {
		return err.Error()
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if nil != err {
		return err.Error()
	}

	return string(body)
}

func ComputeHmac512_384NoDecode(strMessage string, strSecret string) string {
	key := []byte(strSecret)
	h := hmac.New(sha512.New384, key)
	h.Write([]byte(strMessage))

	return hex.EncodeToString(h.Sum(nil))
}