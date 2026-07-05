package domain

import "time"

type Customer struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	Age            int        `json:"age"`
	Address        string     `json:"address"`
	PhoneNumber    string     `json:"phone_number"`
	IdentityNumber string     `json:"identity_number"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
}

type CustomerRequest struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Age            int    `json:"age"`
	Address        string `json:"address"`
	PhoneNumber    string `json:"phone_number"`
	IdentityNumber string `json:"identity_number"`
}
