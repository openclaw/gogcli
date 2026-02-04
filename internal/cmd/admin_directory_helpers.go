package cmd

import "os"

// adminCustomerID returns the customer ID for Admin SDK calls.
// Defaults to "my_customer" (the authenticated admin's domain) but
// can be overridden via the GOG_CUSTOMER_ID environment variable
// for multi-tenant administration.
func adminCustomerID() string {
	if id := os.Getenv("GOG_CUSTOMER_ID"); id != "" {
		return id
	}
	return "my_customer"
}
