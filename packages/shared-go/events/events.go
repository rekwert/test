package events

const (
	PaymentReceived           = "payment.received"
	PaymentFailed             = "payment.failed"
	OrderCreated              = "order.created"
	InstanceProvisionRequested = "instance.provision_requested"
	InstanceStateChanged      = "instance.state_changed"
	InstanceSuspendRequested  = "instance.suspend_requested"
	NotificationSendEmail     = "notification.send_email"
)

type Envelope struct {
	EventID   string `json:"event_id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   any    `json:"payload"`
}
