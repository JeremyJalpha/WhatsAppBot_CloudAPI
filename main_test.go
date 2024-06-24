package main

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	wb "github.com/JeremyJalpha/WhatsAppBot/whatsappbot"
	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

const currentCatalogueID = "WeAreGettingThePig"

func TestParseUpdateOrderCommand(t *testing.T) {
	tests := []struct {
		commandText string
		expected    []wb.MenuIndication
		expectError bool
	}{
		{
			commandText: "update order 9:12, 10: 1x3, 3x2, 2x1, 6:5",
			expected: []wb.MenuIndication{
				{ItemMenuNum: 10, ItemAmount: "1x3, 3x2, 2x1"},
				{ItemMenuNum: 9, ItemAmount: "12"},
				{ItemMenuNum: 6, ItemAmount: "5"},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		result, err := wb.ParseUpdateOrderCommand(test.commandText)
		if (err != nil) != test.expectError {
			t.Errorf("ParseUpdateOrderCommand(%q) error = %v, expectError %v", test.commandText, err, test.expectError)
			continue
		}
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("ParseUpdateOrderCommand(%q) = %v, want %v", test.commandText, result, test.expected)
		}
	}
}

func setupTestDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	createTable := `
	CREATE TABLE customerorder (
		orderID INTEGER PRIMARY KEY,
		cellnumber TEXT NOT NULL,
		catalogueID TEXT NOT NULL,
		orderitems TEXT NOT NULL,
		orderTotal INTEGER DEFAULT 0,
		ispaid BOOLEAN DEFAULT 0,
		datetimedelivered DATETIME,
		isclosed BOOLEAN DEFAULT 0
	);`

	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func TestUpdateOrInsertCurrentOrder(t *testing.T) {
	db, err := setupTestDB()
	assert.NoError(t, err)
	defer db.Close()

	tests := []struct {
		custOrd     wb.CustomerOrder
		expected    wb.OrderItems
		expectError bool
	}{
		{
			custOrd: wb.CustomerOrder{
				OrderID:     12345,
				CellNumber:  "0766140000",
				CatalogueID: currentCatalogueID,
				OrderItems: wb.OrderItems{
					MenuIndications: []wb.MenuIndication{
						{ItemMenuNum: 10, ItemAmount: "1x3, 3x2, 2x1"},
						{ItemMenuNum: 9, ItemAmount: "12"},
						{ItemMenuNum: 6, ItemAmount: "5"},
					},
				},
			},
			expected: wb.OrderItems{
				MenuIndications: []wb.MenuIndication{
					{ItemMenuNum: 10, ItemAmount: "1x3, 3x2, 2x1"},
					{ItemMenuNum: 9, ItemAmount: "12"},
					{ItemMenuNum: 6, ItemAmount: "5"},
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		err = test.custOrd.UpdateOrInsertCurrentOrder(db, test.custOrd.CellNumber, test.custOrd.CatalogueID, test.expected, true)
		assert.NoError(t, err)

		var readOrderItems string
		query := `SELECT orderitems FROM customerorder WHERE orderID = ?`
		err = db.QueryRow(query, test.custOrd.OrderID).Scan(&readOrderItems)
		assert.NoError(t, err)

		// Unmarshal the JSON string back to []OrderItem
		var actual wb.OrderItems
		err = json.Unmarshal([]byte(readOrderItems), &actual)
		assert.NoError(t, err)

		if (err != nil) != test.expectError {
			t.Errorf("UpdateOrInsertCurrentOrder(%q) error = %v, expectError %v", test.custOrd.OrderItems, err, test.expectError)
			continue
		}
		result := wb.OrderItems{MenuIndications: actual.MenuIndications}
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("UpdateOrInsertCurrentOrder(%q) = %v, want %v", test.custOrd.OrderItems, result, test.expected)
		}
	}
}

func TestCheckoutNow(t *testing.T) {
	db, err := setupTestDB()
	assert.NoError(t, err)
	defer db.Close()

	senderNum := "0766140000"
	pymntRtrnBase := "payment_return"
	pymntCnclBase := "payment_canceled"
	returnBaseURL := "/" + pymntRtrnBase
	cancelBaseURL := "/" + pymntCnclBase
	notifyBaseURL := "/payment_notify"
	ItemNamePrefix := "Order"

	HomebaseURL := "https://albacore-inspired-bull.ngrok-free.app"

	MerchantId := "10033925"
	MerchantKey := "ojh77y6acuekb"
	Passphrase := "jt7NOE43FZPnf"
	PfHost := "https://sandbox.payfast.co.za/eng/process"

	checkoutInfo := wb.CheckoutInfo{
		ReturnURL:      HomebaseURL + returnBaseURL,
		CancelURL:      HomebaseURL + cancelBaseURL,
		NotifyURL:      HomebaseURL + notifyBaseURL,
		MerchantId:     MerchantId,
		MerchantKey:    MerchantKey,
		Passphrase:     Passphrase,
		HostURL:        PfHost,
		ItemNamePrefix: ItemNamePrefix,
	}

	tests := []struct {
		userInfo    wb.UserInfo
		custOrd     wb.CustomerOrder
		expected    wb.OrderItems
		expectError bool
	}{
		{
			userInfo: wb.UserInfo{
				CellNumber: senderNum,
				NickName:   wb.NullString{NullString: sql.NullString{String: "testSplurge", Valid: true}},
				Email:      wb.NullString{NullString: sql.NullString{String: "sbtu01@payfast.io", Valid: true}},
			},
			custOrd: wb.CustomerOrder{
				OrderID:     12345,
				CellNumber:  senderNum,
				CatalogueID: currentCatalogueID,
				OrderItems: wb.OrderItems{
					MenuIndications: []wb.MenuIndication{
						{ItemMenuNum: 10, ItemAmount: "1x3, 3x2, 2x1"},
						{ItemMenuNum: 9, ItemAmount: "12"},
						{ItemMenuNum: 6, ItemAmount: "5"},
					},
				},
			},
			expected: wb.OrderItems{
				MenuIndications: []wb.MenuIndication{
					{ItemMenuNum: 10, ItemAmount: "1x3, 3x2, 2x1"},
					{ItemMenuNum: 9, ItemAmount: "12"},
					{ItemMenuNum: 6, ItemAmount: "5"},
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		wb.BeginCheckout(db, test.userInfo, test.custOrd, checkoutInfo, true)
		assert.NoError(t, err)

		// ...

		// if (err != nil) != test.expectError {
		// 	t.Errorf("UpdateOrInsertCurrentOrder(%q) error = %v, expectError %v", test.custOrd.OrderItems, err, test.expectError)
		// 	continue
		// }
		// result := wb.OrderItems{MenuIndications: actual.MenuIndications}
		// if !reflect.DeepEqual(result, test.expected) {
		// 	t.Errorf("UpdateOrInsertCurrentOrder(%q) = %v, want %v", test.custOrd.OrderItems, result, test.expected)
		// }
	}
}
