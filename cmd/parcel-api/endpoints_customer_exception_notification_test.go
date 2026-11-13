package main

func init() {
	registerUnwired(customerExceptionNotificationWiring{notifier: unwiredCustomerExceptionNotification{}})
}
