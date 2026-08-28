package tbank

import "strings"

// Receipt is sent to T-Bank when fiscalization (online cash register) is enabled on the terminal.
type Receipt struct {
	Email    string
	Taxation string
	Items    []ReceiptItem
}

type ReceiptItem struct {
	Name          string
	Price         int64
	Quantity      int64
	Amount        int64
	Tax           string
	PaymentMethod string
	PaymentObject string
}

func NewTopupReceipt(email, taxation, itemTax, itemName string, amountKopecks int64) *Receipt {
	email = strings.TrimSpace(email)
	taxation = strings.TrimSpace(taxation)
	itemTax = strings.TrimSpace(itemTax)
	itemName = strings.TrimSpace(itemName)
	if taxation == "" || amountKopecks <= 0 || itemName == "" {
		return nil
	}
	if itemTax == "" {
		itemTax = "none"
	}
	return &Receipt{
		Email:    email,
		Taxation: taxation,
		Items: []ReceiptItem{
			{
				Name:          itemName,
				Price:         amountKopecks,
				Quantity:      1,
				Amount:        amountKopecks,
				Tax:           itemTax,
				PaymentMethod: "advance",
				PaymentObject: "payment",
			},
		},
	}
}

func (r *Receipt) toMap() map[string]any {
	if r == nil {
		return nil
	}
	out := map[string]any{
		"Taxation": r.Taxation,
	}
	if r.Email != "" {
		out["Email"] = r.Email
	}
	items := make([]map[string]any, 0, len(r.Items))
	for _, item := range r.Items {
		items = append(items, map[string]any{
			"Name":          item.Name,
			"Price":         item.Price,
			"Quantity":      item.Quantity,
			"Amount":        item.Amount,
			"Tax":           item.Tax,
			"PaymentMethod": item.PaymentMethod,
			"PaymentObject": item.PaymentObject,
		})
	}
	out["Items"] = items
	return out
}
