package main

type ShipDro struct {
	Name    string `gorm:"primaryKey"`
	Ship    string
	Prefix  string //URL Prefix
	Enabled bool
}
