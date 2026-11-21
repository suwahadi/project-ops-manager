package notify

import (
	"fmt"
	"strings"
	"time"
)

type Message struct {
	Subject string
	Body    string
}

type Reminder struct {
	AssetName    string
	TypeName     string
	Owner        string
	Provider     string
	Expiration   time.Time
	DaysLeft     int
	CostPrice    string
	SellingPrice string
	Link         string
}

func dayWord(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

func BuildReminder(r Reminder) Message {
	subject := fmt.Sprintf("Renewal in %s: %s (%s)", dayWord(r.DaysLeft), r.AssetName, r.TypeName)
	selling := r.SellingPrice
	if selling == "" {
		selling = "—"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s expires in %s.\n\n", r.AssetName, dayWord(r.DaysLeft))
	fmt.Fprintf(&b, "Asset: %s\n", r.AssetName)
	fmt.Fprintf(&b, "Type: %s\n", r.TypeName)
	fmt.Fprintf(&b, "Owner: %s\n", r.Owner)
	fmt.Fprintf(&b, "Provider: %s\n", r.Provider)
	fmt.Fprintf(&b, "Expiration date: %s\n", r.Expiration.Format("02 Jan 2006"))
	fmt.Fprintf(&b, "Days left: %d\n", r.DaysLeft)
	fmt.Fprintf(&b, "Cost price: %s\n", r.CostPrice)
	fmt.Fprintf(&b, "Selling price: %s\n", selling)
	fmt.Fprintf(&b, "\nOpen asset: %s\n", r.Link)
	return Message{Subject: subject, Body: b.String()}
}

func TestMessage(channel string) Message {
	return Message{
		Subject: "Ops Manager test notification",
		Body:    "This is a test " + channel + " notification from Ops Manager. Reminder delivery is configured correctly.",
	}
}
