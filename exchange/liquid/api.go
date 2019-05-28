package liquid

// Copyright (c) 2015-2019 Bitontop Technologies Inc.
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitontop/gored/coin"
	"github.com/bitontop/gored/exchange"
	"github.com/bitontop/gored/pair"
)

const (
	API_URL string = "https://api.liquid.com"
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
func (e *Liquid) GetCoinsData() error {
	pairsData := PairsData{}

	strRequestUrl := "/products"
	strUrl := API_URL + strRequestUrl

	jsonCurrencyReturn := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonCurrencyReturn), &pairsData); err != nil {
		return fmt.Errorf("%s Get Coins Json Unmarshal Err: %v %v", e.GetName(), err, jsonCurrencyReturn)
	}

	for _, data := range pairsData {
		base := &coin.Coin{}
		target := &coin.Coin{}
		switch e.Source {
		case exchange.EXCHANGE_API:
			base = coin.GetCoin(data.QuotedCurrency)
			if base == nil {
				base = &coin.Coin{}
				base.Code = data.QuotedCurrency
				coin.AddCoin(base)
			}
			target = coin.GetCoin(data.BaseCurrency)
			if target == nil {
				target = &coin.Coin{}
				target.Code = data.BaseCurrency
				coin.AddCoin(target)
			}
		case exchange.JSON_FILE:
			base = e.GetCoinBySymbol(data.QuotedCurrency)
			target = e.GetCoinBySymbol(data.BaseCurrency)
		}

		if base != nil {
			coinConstraint := &exchange.CoinConstraint{
				CoinID:       base.ID,
				Coin:         base,
				ExSymbol:     data.QuotedCurrency,
				ChianType:    exchange.MAINNET,
				TxFee:        DEFAULT_TXFEE,
				Withdraw:     DEFAULT_WITHDRAW,
				Deposit:      DEFAULT_DEPOSIT,
				Confirmation: DEFAULT_CONFIRMATION,
				Listed:       DEFAULT_LISTED,
			}
			e.SetCoinConstraint(coinConstraint)
		}

		if target != nil {
			coinConstraint := &exchange.CoinConstraint{
				CoinID:       target.ID,
				Coin:         target,
				ExSymbol:     data.BaseCurrency,
				ChianType:    exchange.MAINNET,
				TxFee:        DEFAULT_TXFEE,
				Withdraw:     DEFAULT_WITHDRAW,
				Deposit:      DEFAULT_DEPOSIT,
				Confirmation: DEFAULT_CONFIRMATION,
				Listed:       DEFAULT_LISTED,
			}
			e.SetCoinConstraint(coinConstraint)
		}
	}
	return nil
}

/* GetPairsData - Get Pairs Information (If API provide)
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Modify API Path(strRequestUrl)*/
func (e *Liquid) GetPairsData() error {
	pairsData := PairsData{}

	strRequestUrl := "/products"
	strUrl := API_URL + strRequestUrl

	jsonSymbolsReturn := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonSymbolsReturn), &pairsData); err != nil {
		return fmt.Errorf("%s Get Pairs Json Unmarshal Err: %v %v", e.GetName(), err, jsonSymbolsReturn)
	}

	for _, data := range pairsData {
		p := &pair.Pair{}
		switch e.Source {
		case exchange.EXCHANGE_API:
			base := coin.GetCoin(data.QuotedCurrency)
			target := coin.GetCoin(data.BaseCurrency)
			if base != nil && target != nil {
				p = pair.GetPair(base, target)
			}
		case exchange.JSON_FILE:
			p = e.GetPairBySymbol(data.CurrencyPairCode)
		}
		if p != nil {
			makerfee, _ := strconv.ParseFloat(data.MakerFee, 64)
			takerfee, _ := strconv.ParseFloat(data.TakerFee, 64)
			pairConstraint := &exchange.PairConstraint{
				PairID:      p.ID,
				Pair:        p,
				ExSymbol:    data.ID,
				MakerFee:    makerfee,
				TakerFee:    takerfee,
				LotSize:     DEFAULT_LOT_SIZE,
				PriceFilter: DEFAULT_PRICE_FILTER,
				Listed:      !data.Disabled,
			}
			e.SetPairConstraint(pairConstraint)
		}
	}
	return nil
}

/*Get Pair Market Depth
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Get Exchange Pair Code ex. symbol := e.GetPairCode(p)
Step 4: Modify API Path(strRequestUrl)
Step 5: Add Params - Depend on API request
Step 6: Convert the response to Standard Maker struct*/
func (e *Liquid) OrderBook(pair *pair.Pair) (*exchange.Maker, error) {
	orderBook := OrderBook{}
	symbol := e.GetSymbolByPair(pair)

	strRequestUrl := fmt.Sprintf("/products/%s/price_levels", symbol)
	strUrl := API_URL + strRequestUrl

	maker := &exchange.Maker{}
	maker.WorkerIP = exchange.GetExternalIP()
	maker.BeforeTimestamp = float64(time.Now().UnixNano() / 1e6)

	jsonOrderbook := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonOrderbook), &orderBook); err != nil {
		return nil, fmt.Errorf("%s Get Orderbook Json Unmarshal Err: %v %v", e.GetName(), err, jsonOrderbook)
	}

	maker.AfterTimestamp = float64(time.Now().UnixNano() / 1e6)
	var err error
	for _, bid := range orderBook.BuyPriceLevels {
		var buydata exchange.Order

		//Modify according to type and structure
		buydata.Rate, err = strconv.ParseFloat(bid[0], 64)
		if err != nil {
			return nil, err
		}
		buydata.Quantity, err = strconv.ParseFloat(bid[1], 64)
		if err != nil {
			return nil, err
		}

		maker.Bids = append(maker.Bids, buydata)
	}
	for _, ask := range orderBook.SellPriceLevels {
		var selldata exchange.Order

		//Modify according to type and structure
		selldata.Rate, err = strconv.ParseFloat(ask[0], 64)
		if err != nil {
			return nil, err
		}
		selldata.Quantity, err = strconv.ParseFloat(ask[1], 64)
		if err != nil {
			return nil, err
		}

		maker.Asks = append(maker.Asks, selldata)
	}
	return maker, nil
}

/*************** Private API ***************/
func (e *Liquid) UpdateAllBalances() {
	if e.API_KEY == "" || e.API_SECRET == "" {
		log.Printf("%s API Key or Secret Key are nil.", e.GetName())
		return
	}

	accountBalance := AccountBalances{}
	strRequest := "/accounts/balance"

	jsonBalanceReturn := e.ApiKeyRequest("GET", nil, strRequest)
	if err := json.Unmarshal([]byte(jsonBalanceReturn), &accountBalance); err != nil {
		log.Printf("%s UpdateAllBalances Json Unmarshal Err: %v %v", e.GetName(), err, jsonBalanceReturn)
		return
	}

	for _, v := range accountBalance {
		c := e.GetCoinBySymbol(v.Currency)
		if c != nil {
			available, err := strconv.ParseFloat(v.Balance, 64)
			if err != nil {
				log.Printf("%s free balance parse Err: %v %v", e.GetName(), err, v.Balance)
			}
			balanceMap.Set(c.Code, available)
		}
	}
}

func (e *Liquid) Withdraw(coin *coin.Coin, quantity float64, addr, tag string) bool {
	return false
}

func (e *Liquid) LimitSell(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return nil, fmt.Errorf("%s API Key or Secret Key are nil", e.GetName())
	}

	placeOrder := PlaceOrder{}
	strRequest := "/orders/"

	mapParams := make(map[string]interface{})
	mapParams["order_type"] = "limit"
	mapParams["product_id"] = e.GetSymbolByPair(pair)
	mapParams["side"] = "sell"
	mapParams["quantity"] = fmt.Sprint(quantity)
	mapParams["price"] = fmt.Sprint(rate)

	jsonPlaceReturn := e.ApiKeyRequest("POST", mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonPlaceReturn), &placeOrder); err != nil {
		return nil, fmt.Errorf("%s LimitSell Json Unmarshal Err: %v %v", e.GetName(), err, jsonPlaceReturn)
	}

	order := &exchange.Order{
		Pair:         pair,
		OrderID:      strconv.Itoa(placeOrder.ID),
		Rate:         rate,
		Quantity:     quantity,
		Side:         "Sell",
		Status:       exchange.New,
		JsonResponse: jsonPlaceReturn,
	}

	return order, nil
}

func (e *Liquid) LimitBuy(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return nil, fmt.Errorf("%s API Key or Secret Key are nil", e.GetName())
	}

	placeOrder := PlaceOrder{}
	strRequest := "/orders/"

	mapParams := make(map[string]interface{})
	mapParams["order_type"] = "limit"
	mapParams["product_id"] = e.GetSymbolByPair(pair)
	mapParams["side"] = "buy"
	mapParams["quantity"] = fmt.Sprint(quantity)
	mapParams["price"] = fmt.Sprint(rate)

	jsonPlaceReturn := e.ApiKeyRequest("POST", mapParams, strRequest)
	if err := json.Unmarshal([]byte(jsonPlaceReturn), &placeOrder); err != nil {
		return nil, fmt.Errorf("%s LimitBuy Json Unmarshal Err: %v %v", e.GetName(), err, jsonPlaceReturn)
	}

	order := &exchange.Order{
		Pair:         pair,
		OrderID:      strconv.Itoa(placeOrder.ID),
		Rate:         rate,
		Quantity:     quantity,
		Side:         "Buy",
		Status:       exchange.New,
		JsonResponse: jsonPlaceReturn,
	}

	return order, nil
}

func (e *Liquid) OrderStatus(order *exchange.Order) error {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return fmt.Errorf("%s API Key or Secret Key are nil", e.GetName())
	}

	orderStatus := OrderStatus{}
	strRequest := fmt.Sprintf("/orders/%s", order.OrderID)

	jsonOrderStatus := e.ApiKeyRequest("GET", nil, strRequest)
	if err := json.Unmarshal([]byte(jsonOrderStatus), &orderStatus); err != nil {
		return fmt.Errorf("%s OrderStatus Json Unmarshal Err: %v %v", e.GetName(), err, jsonOrderStatus)
	}

	order.StatusMessage = jsonOrderStatus
	if strconv.Itoa(orderStatus.ID) == order.OrderID {
		switch orderStatus.Status {
		case "live":
			order.Status = exchange.New
		case "partially_filled":
			order.Status = exchange.Partial
		case "cancelled":
			order.Status = exchange.Canceled
		case "filled":
			order.Status = exchange.Filled
		default:
			order.Status = exchange.Other
		}
	}

	return nil
}

func (e *Liquid) ListOrders() ([]*exchange.Order, error) {
	return nil, nil
}

func (e *Liquid) CancelOrder(order *exchange.Order) error {
	if e.API_KEY == "" || e.API_SECRET == "" {
		return fmt.Errorf("%s API Key or Secret Key are nil", e.GetName())
	}

	cancelOrder := CancelOrder{}
	strRequest := fmt.Sprintf("/orders/%s/cancel", order.OrderID)

	jsonCancelOrder := e.ApiKeyRequest("PUT", nil, strRequest)
	if err := json.Unmarshal([]byte(jsonCancelOrder), &cancelOrder); err != nil {
		return fmt.Errorf("%s CancelOrder Json Unmarshal Err: %v %v", e.GetName(), err, jsonCancelOrder)
	}

	order.Status = exchange.Canceling
	order.CancelStatus = jsonCancelOrder

	return nil
}

func (e *Liquid) CancelAllOrder() error {
	return nil
}

/*************** Signature Http Request ***************/
/*Method: API Request and Signature is required
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Create mapParams Depend on API Signature request
Step 3: Add HttpGetRequest below strUrl if API has different requests*/
func (e *Liquid) ApiKeyRequest(strMethod string, mapParams map[string]interface{}, strRequestPath string) string {
	timestamp := strconv.FormatInt(time.Now().UnixNano()/1e6, 10) //time.Now().UTC().Format("2006-01-02T15:04:05")
	strUrl := API_URL + strRequestPath

	jsonParams := ""
	byteParams, err := json.Marshal(mapParams)
	if nil != mapParams {
		jsonParams = string(byteParams)
	}

	//Signature Request Params
	headerParams := make(map[string]interface{})
	headerParams["alg"] = "HS256"
	headerParams["typ"] = "JWT"

	payloadParams := make(map[string]interface{})
	payloadParams["path"] = strRequestPath
	payloadParams["nonce"] = timestamp
	payloadParams["token_id"] = e.API_KEY

	signature := ""

	// 64 encode header & payload:
	header := ""
	bytesHead, err := json.Marshal(headerParams)
	if nil != headerParams {
		header = string(bytesHead)
	}
	header64 := base64.StdEncoding.EncodeToString([]byte(header))

	payload := ""
	bytesParams, err := json.Marshal(payloadParams)
	if nil != payloadParams {
		payload = string(bytesParams)
	}
	payload64 := base64.StdEncoding.EncodeToString([]byte(payload))
	signature = exchange.ComputeHmac256URL(header64+"."+payload64, e.API_SECRET)

	// final signature
	fullSignature := header64 + "." + payload64 + "." + signature

	httpClient := &http.Client{}
	request, err := http.NewRequest(strMethod, strUrl, strings.NewReader(jsonParams))

	if nil != err {
		return err.Error()
	}
	request.Header.Add("X-Quoine-API-Version", "2")
	request.Header.Add("X-Quoine-Auth", fullSignature)
	request.Header.Add("Content-Type", "application/json;charset=utf-8")

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
