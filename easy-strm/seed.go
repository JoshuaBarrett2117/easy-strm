package main

import (
	"fmt"
)

// SeedAdmin ensures that the admin user exists
func SeedAdmin() error {
	const adminName = "admin"
	// MD5 hash of "admin"
	const adminPasswordMD5 = "21232f297a57a5a743894a0e4a801fc3"

	user, err := GetUserByName(adminName)
	if err == nil && user != nil {
		Info("Admin user '%s' already exists.", adminName)
		return nil
	}

	Info("Admin user not found. Creating default admin user...")
	_, err = CreateUser(adminName, adminPasswordMD5)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %v", err)
	}

	Info("Admin user '%s' created successfully.", adminName)
	return nil
}
