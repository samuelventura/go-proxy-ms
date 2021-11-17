package main

type ShipDro struct {
	Name    string `gorm:"primaryKey"`
	Prefix  string //URL Prefix
	Enabled bool
}
