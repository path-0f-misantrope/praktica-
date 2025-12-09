package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
)

func main() {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:zxcqwe123@localhost:5432/practice")
	if err != nil {
		log.Fatalf("подключение к бд упало: %v", err)
	}
	defer conn.Close(ctx)

	file, err := excelize.OpenFile("Products_import.xlsx")
	if err != nil {
		log.Fatalf("ошибка открытия файла: %v", err)
	}
	defer file.Close()

	sheet := file.GetSheetList()[0]
	rows, err := file.GetRows(sheet)
	if err != nil {
		log.Fatalf("ошибка чтения строк: %v", err)
	}

	successCount := 0
	errorCount := 0

	for i, row := range rows {
		// Пропускаем заголовок
		if i == 0 {
			continue
		}

		// Проверяем что строка не пустая
		if len(row) < 5 {
			log.Printf("Строка %d: недостаточно колонок (нужно минимум 5)", i+1)
			errorCount++
			continue
		}

		// Парсим данные из Excel
		productName := strings.TrimSpace(row[1])
		materialName := strings.TrimSpace(row[4])
		article := strings.TrimSpace(row[2])
		typeName := strings.TrimSpace(row[0])

		minPrice, err := parseDecimal(row[3])
		if err != nil {
			log.Printf("Строка %d: ошибка парсинга цены '%s': %v", i+1, row[4], err)
			errorCount++
			continue
		}

		// Получаем material_id по имени материала
		materialID, err := getMaterialID(ctx, conn, materialName)
		if err != nil {
			log.Printf("Строка %d: %v", i+1, err)
			errorCount++
			continue
		}

		// Получаем type_id по имени типа
		typeID, err := getTypeID(ctx, conn, typeName)
		if err != nil {
			log.Printf("Строка %d: %v", i+1, err)
			errorCount++
			continue
		}

		// Вставляем продукт
		_, err = conn.Exec(ctx, `
			INSERT INTO products (product_name, material_id, type_id, min_price, article)
			VALUES ($1, $2, $3, $4, $5)
		`, productName, materialID, typeID, minPrice, article)

		if err != nil {
			log.Printf("Строка %d: ошибка вставки: %v", i+1, err)
			errorCount++
			continue
		}

		successCount++
	}

	fmt.Printf("\n✅ Импорт завершён!\n")
	fmt.Printf("Успешно: %d\n", successCount)
	fmt.Printf("Ошибок: %d\n", errorCount)
}

// getMaterialID находит ID материала по его имени
func getMaterialID(ctx context.Context, conn *pgx.Conn, materialName string) (int, error) {
	var id int

	query := "SELECT id FROM materials WHERE LOWER(material_name) = LOWER($1)"
	err := conn.QueryRow(ctx, query, materialName).Scan(&id)

	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("материал '%s' не найден в базе", materialName)
	}
	if err != nil {
		return 0, fmt.Errorf("ошибка поиска материала: %v", err)
	}

	return id, nil
}

// getTypeID находит ID типа по его имени
func getTypeID(ctx context.Context, conn *pgx.Conn, typeName string) (int, error) {
	var id int

	query := "SELECT id FROM products_types WHERE LOWER(type_name) = LOWER($1)"
	err := conn.QueryRow(ctx, query, typeName).Scan(&id)

	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("тип '%s' не найден в базе", typeName)
	}
	if err != nil {
		return 0, fmt.Errorf("ошибка поиска типа: %v", err)
	}

	return id, nil
}

// parseDecimal парсит число (цену)
func parseDecimal(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")

	if s == "" {
		return 0, nil
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("невалидное число: %v", err)
	}

	return value, nil
}
