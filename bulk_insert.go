package main

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"strings"
	"time"
	"errors"
	"reflect"
)

// Insert multiple records at once
func BulkInsert(db *gorm.DB, objects []interface{}, chunkSize int) error {
	// Split records with specified size not to exceed Database parameter limit
	for _, objSet := range splitObjects(objects, chunkSize) {
		if err := insertObjSet(db, objSet); err != nil {
			return err
		}
	}
	return nil
}

func insertObjSet(db *gorm.DB, objects []interface{}) error {
	if len(objects) == 0 {
		return nil
	}

	firstAttrs, err := extractMapValue(objects[0])
	if err != nil {
		return err
	}

	attrSize := len(firstAttrs)

	// Scope to eventually run SQL
	mainScope := db.NewScope(objects[0])
	// Store placeholders for embedding variables
	placeholders := make([]string, 0, attrSize)

	// Replace with database column name
	dbColumns := make([]string, 0, attrSize)
	for _, key := range sortedKeys(firstAttrs) {
		dbColumns = append(dbColumns, gorm.ToColumnName(key))
	}

	for _, obj := range objects {
		objAttrs, err := extractMapValue(obj)
		if err != nil {
			return err
		}

		// If object sizes are different, SQL statement loses consistency
		if len(objAttrs) != attrSize {
			return errors.New("attribute sizes are inconsistent")
		}

		scope := db.NewScope(obj)

		// Append variables
		variables := make([]string, 0, attrSize)
		for _, key := range sortedKeys(objAttrs) {
			variables = append(variables, scope.AddToVars(objAttrs[key]))
		}

		valueQuery := "(" + strings.Join(variables, ", ") + ")"
		placeholders = append(placeholders, valueQuery)

		// Also append variables to mainScope
		mainScope.SQLVars = append(mainScope.SQLVars, scope.SQLVars...)
	}

	mainScope.Raw(fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		mainScope.QuotedTableName(),
		strings.Join(dbColumns, ", "),
		strings.Join(placeholders, ", "),
	))

	return db.Exec(mainScope.SQL, mainScope.SQLVars...).Error
}

// Obtain columns and values required for insert from interface
func extractMapValue(value interface{}) (map[string]interface{}, error) {
	if reflect.ValueOf(value).Kind() != reflect.Struct {
		return nil, errors.New("value must be kind of Struct")
	}

	var attrs = map[string]interface{}{}

	for _, field := range (&gorm.Scope{Value: value}).Fields() {
		// Exclude relational record because it's not directly contained in database columns
		if field.StructField.Relationship == nil && !field.IsIgnored && !field.IsPrimaryKey {
			if field.Struct.Name == "CreatedAt" || field.Struct.Name == "UpdatedAt" {
				attrs[field.DBName] = time.Now()
			} else if field.StructField.HasDefaultValue && field.IsBlank {
				// If default value presents and field is empty, assign a default value
				if val, ok := field.TagSettingsGet("DEFAULT"); ok {
					attrs[field.DBName] = val
				} else {
					attrs[field.DBName] = field.Field.Interface()
				}
			} else {
				attrs[field.DBName] = field.Field.Interface()
			}
		}
	}
	return attrs, nil
}