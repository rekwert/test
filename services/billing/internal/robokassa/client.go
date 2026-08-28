package robokassa

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const paymentBaseURL = "https://auth.robokassa.ru/Merchant/Index.aspx"

type Client struct {
	merchantLogin string
	password1     string
	password2     string
	testPassword1 string
	testPassword2 string
	testMode      bool
}

func NewClient(merchantLogin, password1, password2, testPassword1, testPassword2 string, testMode bool) *Client {
	return &Client{
		merchantLogin: strings.TrimSpace(merchantLogin),
		password1:     password1,
		password2:     password2,
		testPassword1: testPassword1,
		testPassword2: testPassword2,
		testMode:      testMode,
	}
}

func (c *Client) password1ForPayment() string {
	if c.testMode && c.testPassword1 != "" {
		return c.testPassword1
	}
	return c.password1
}

func (c *Client) Password2ForResult(isTest bool) string {
	if isTest && c.testPassword2 != "" {
		return c.testPassword2
	}
	return c.password2
}

func PaymentSignature(merchantLogin, outSum, invID, password1 string) string {
	raw := merchantLogin + ":" + outSum + ":" + invID + ":" + password1
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func ResultSignature(outSum, invID, password2 string) string {
	raw := outSum + ":" + invID + ":" + password2
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type PaymentRequest struct {
	InvID       int64
	Amount      float64
	Description string
	SuccessURL  string
	FailURL     string
}

func (c *Client) PaymentURL(req PaymentRequest) string {
	outSum := FormatAmount(req.Amount)
	invID := strconv.FormatInt(req.InvID, 10)
	sig := PaymentSignature(c.merchantLogin, outSum, invID, c.password1ForPayment())

	q := url.Values{}
	q.Set("MerchantLogin", c.merchantLogin)
	q.Set("OutSum", outSum)
	q.Set("InvId", invID)
	q.Set("Description", req.Description)
	q.Set("SignatureValue", sig)
	if c.testMode {
		q.Set("IsTest", "1")
	}
	if req.SuccessURL != "" {
		q.Set("SuccessURL", req.SuccessURL)
	}
	if req.FailURL != "" {
		q.Set("FailURL", req.FailURL)
	}
	return paymentBaseURL + "?" + q.Encode()
}

func FormatAmount(amount float64) string {
	return fmt.Sprintf("%.2f", amount)
}

func VerifyResult(outSum, invID, signature, password2 string) bool {
	got := strings.ToLower(strings.TrimSpace(signature))
	expect := strings.ToLower(ResultSignature(strings.TrimSpace(outSum), strings.TrimSpace(invID), password2))
	return got != "" && got == expect
}

func ParseAmount(outSum string) (float64, error) {
	outSum = strings.TrimSpace(strings.ReplaceAll(outSum, ",", "."))
	return strconv.ParseFloat(outSum, 64)
}

func ParseInvID(invID string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(invID), 10, 64)
}
