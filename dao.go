package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/samuelventura/go-tree"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type daoDso struct {
	mutex *sync.Mutex
	db    *gorm.DB
}

type Dao interface {
	Close() error
	CountShips() int64
	CountEnabledShips() int64
	CountDisabledShips() int64
	AddShip(name string, ship string, prefix string) error
	GetShip(name string) (*ShipDro, error)
	EnableShip(name string, enabled bool) error
}

func dialector(node tree.Node) gorm.Dialector {
	driver := node.GetValue("driver").(string)
	source := node.GetValue("source").(string)
	switch driver {
	case "sqlite":
		return sqlite.Open(source)
	case "postgres":
		return postgres.Open(source)
	}
	log.Fatalf("unknown driver %s", driver)
	return nil
}

func NewDao(node tree.Node) Dao {
	mode := logger.Default.LogMode(logger.Silent)
	config := &gorm.Config{Logger: mode}
	dialector := dialector(node)
	db, err := gorm.Open(dialector, config)
	if err != nil {
		log.Panic(err)
	}
	err = db.AutoMigrate(&ShipDro{})
	if err != nil {
		log.Panic(err)
	}
	return &daoDso{&sync.Mutex{}, db}
}

func (dso *daoDso) Close() error {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	sqlDB, err := dso.db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	if err != nil {
		return err
	}
	return nil
}

func (dso *daoDso) CountShips() int64 {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	count := int64(0)
	result := dso.db.Model(&ShipDro{}).Count(&count)
	if result.Error != nil {
		log.Panic(result.Error)
	}
	return count
}

func (dso *daoDso) CountEnabledShips() int64 {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	count := int64(0)
	result := dso.db.Model(&ShipDro{}).Where("enabled = ?", true).Count(&count)
	if result.Error != nil {
		log.Panic(result.Error)
	}
	return count
}

func (dso *daoDso) CountDisabledShips() int64 {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	count := int64(0)
	result := dso.db.Model(&ShipDro{}).Where("enabled != ?", true).Count(&count)
	if result.Error != nil {
		log.Panic(result.Error)
	}
	return count
}

func (dso *daoDso) AddShip(name string, ship string, prefix string) error {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	dro := &ShipDro{Name: name, Ship: ship, Prefix: prefix}
	result := dso.db.Create(dro)
	return result.Error
}

func (dso *daoDso) GetShip(name string) (*ShipDro, error) {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	dro := &ShipDro{}
	result := dso.db.
		Where("name = ?", name).
		First(dro)
	return dro, result.Error
}

func (dso *daoDso) EnableShip(name string, enabled bool) error {
	dso.mutex.Lock()
	defer dso.mutex.Unlock()
	result := dso.db.Model(&ShipDro{}).
		Where("name = ?", name).Update("enabled", enabled)
	if result.Error == nil && result.RowsAffected != 1 {
		return fmt.Errorf("ship not found")
	}
	return result.Error
}
