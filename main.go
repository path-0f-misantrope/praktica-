package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============ МОДЕЛИ ============
type ProductWithTime struct {
	ID                  int     `json:"id"`
	ProductName         string  `json:"product_name"`
	MaterialName        string  `json:"material_name"`
	TypeName            string  `json:"type_name"`
	MinPrice            float64 `json:"min_price"`
	Article             string  `json:"article"`
	TotalProductionTime float64 `json:"total_production_time"`
}

// ============ СЛОЙ БД (repository) ============

// GetAllProducts получает все продукты со временем производства
func GetAllProducts(ctx context.Context, pool *pgxpool.Pool) ([]ProductWithTime, error) {
	query := `
		SELECT 
			p.id,
			p.product_name,
			m.material_name,
			pt.type_name,
			p.min_price,
			p.article,
			COALESCE(SUM(pw.production_time), 0) as total_production_time
		FROM products p
		JOIN materials m ON p.material_id = m.id
		JOIN products_types pt ON p.type_id = pt.id
		LEFT JOIN products_workshop pw ON pw.product_id = p.id
		GROUP BY p.id, p.product_name, m.material_name, pt.type_name, p.min_price, p.article
		ORDER BY p.id
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []ProductWithTime
	for rows.Next() {
		var p ProductWithTime
		err := rows.Scan(
			&p.ID,
			&p.ProductName,
			&p.MaterialName,
			&p.TypeName,
			&p.MinPrice,
			&p.Article,
			&p.TotalProductionTime,
		)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

// GetProductByID получает один продукт по ID со временем производства
func GetProductByID(ctx context.Context, pool *pgxpool.Pool, id int) (*ProductWithTime, error) {
	query := `
		SELECT 
			p.id,
			p.product_name,
			m.material_name,
			pt.type_name,
			p.min_price,
			p.article,
			COALESCE(SUM(pw.production_time), 0) as total_production_time
		FROM products p
		JOIN materials m ON p.material_id = m.id
		JOIN products_types pt ON p.type_id = pt.id
		LEFT JOIN products_workshop pw ON pw.product_id = p.id
		WHERE p.id = $1
		GROUP BY p.id, p.product_name, m.material_name, pt.type_name, p.min_price, p.article
	`

	var p ProductWithTime
	err := pool.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.ProductName,
		&p.MaterialName,
		&p.TypeName,
		&p.MinPrice,
		&p.Article,
		&p.TotalProductionTime,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// CreateProductInput - данные для создания продукта
type CreateProductInput struct {
	ProductName string  `json:"product_name" binding:"required"`
	MaterialID  int     `json:"material_id" binding:"required"`
	TypeID      int     `json:"type_id" binding:"required"`
	MinPrice    float64 `json:"min_price"`
	Article     string  `json:"article"`
}

// WorkshopInput - данные о цехе для продукта
type WorkshopInput struct {
	WorkshopID     int     `json:"workshop_id" binding:"required"`
	ProductionTime float64 `json:"production_time" binding:"required"`
}

// CreateProductWithWorkshopsInput - данные для создания продукта с цехами
type CreateProductWithWorkshopsInput struct {
	ProductName string          `json:"product_name" binding:"required"`
	MaterialID  int             `json:"material_id" binding:"required"`
	TypeID      int             `json:"type_id" binding:"required"`
	MinPrice    float64         `json:"min_price"`
	Article     string          `json:"article"`
	Workshops   []WorkshopInput `json:"workshops"` // массив цехов
}

// CreateProduct создаёт новый продукт и возвращает его ID
func CreateProduct(ctx context.Context, pool *pgxpool.Pool, input CreateProductInput) (int, error) {
	query := `
		INSERT INTO products (product_name, material_id, type_id, min_price, article)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	var productID int
	err := pool.QueryRow(ctx, query,
		input.ProductName,
		input.MaterialID,
		input.TypeID,
		input.MinPrice,
		input.Article,
	).Scan(&productID)

	if err != nil {
		return 0, err
	}

	return productID, nil
}

func GetAllWorkshops(ctx context.Context, pool *pgxpool.Pool) ([]Workshop, error) {
	query := `SELECT id, name FROM workshops ORDER BY name`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workshops []Workshop
	for rows.Next() {
		var w Workshop
		err := rows.Scan(&w.ID, &w.Name)
		if err != nil {
			return nil, err
		}
		workshops = append(workshops, w)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return workshops, nil
}

// CreateProductWithWorkshops создаёт продукт И связи с цехами в одной транзакции
func CreateProductWithWorkshops(ctx context.Context, pool *pgxpool.Pool, input CreateProductWithWorkshopsInput) (int, error) {
	// Начинаем транзакцию
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback(ctx) // откатываем если что-то пойдёт не так

	// Запрос 1: Создаём продукт
	query1 := `
		INSERT INTO products (product_name, material_id, type_id, min_price, article)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	var productID int
	err = tx.QueryRow(ctx, query1,
		input.ProductName,
		input.MaterialID,
		input.TypeID,
		input.MinPrice,
		input.Article,
	).Scan(&productID)

	if err != nil {
		return 0, fmt.Errorf("ошибка создания продукта: %w", err)
	}

	// Запрос 2: Добавляем связи с цехами (если есть)
	if len(input.Workshops) > 0 {
		query2 := `
			INSERT INTO products_workshop (product_id, workshop_id, production_time)
			VALUES ($1, $2, $3)
		`

		for _, workshop := range input.Workshops {
			_, err = tx.Exec(ctx, query2, productID, workshop.WorkshopID, workshop.ProductionTime)
			if err != nil {
				return 0, fmt.Errorf("ошибка добавления цеха %d: %w", workshop.WorkshopID, err)
			}
		}
	}

	// Коммитим транзакцию
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("ошибка коммита транзакции: %w", err)
	}

	return productID, nil
}
func DeleteById(ctx context.Context, pool *pgxpool.Pool, product_id int) error {
	query := `DELETE FROM products WHERE id = $1`
	result, err := pool.Exec(ctx, query, product_id)
	if result.RowsAffected() == 0 {
		return fmt.Errorf("ни одной строки не удалилось ")
	}
	if err != nil {
		return fmt.Errorf("ошибка удаления ")
	}
	return nil
}

// ============ СЛОЙ HTTP (handlers) ============

type Server struct {
	pool *pgxpool.Pool
}

// GET /api/products
func (s *Server) GetProductsHandler(c *gin.Context) {
	products, err := GetAllProducts(c.Request.Context(), s.pool)
	if err != nil {
		log.Printf("Ошибка получения продуктов: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Не удалось получить список продуктов",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"count":    len(products),
	})
}

// GET /api/products/:id
func (s *Server) GetProductByIDHandler(c *gin.Context) {
	id := c.Param("id")

	var productID int
	if _, err := fmt.Sscanf(id, "%d", &productID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверный ID продукта",
		})
		return
	}

	product, err := GetProductByID(c.Request.Context(), s.pool, productID)
	if err != nil {
		log.Printf("Ошибка получения продукта: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Не удалось получить продукт",
		})
		return
	}

	if product == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Продукт не найден",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

// POST /api/products
func (s *Server) CreateProductHandler(c *gin.Context) {
	var input CreateProductInput

	// Валидация JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверные данные: " + err.Error(),
		})
		return
	}

	// Создание продукта
	productID, err := CreateProduct(c.Request.Context(), s.pool, input)
	if err != nil {
		log.Printf("Ошибка создания продукта: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Не удалось создать продукт",
		})
		return
	}

	// Получаем созданный продукт
	product, err := GetProductByID(c.Request.Context(), s.pool, productID)
	if err != nil {
		log.Printf("Ошибка получения созданного продукта: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Продукт создан, но не удалось получить его данные",
		})
		return
	}

	c.JSON(http.StatusCreated, product)
}

// POST /api/products/with-workshops
func (s *Server) CreateProductWithWorkshopsHandler(c *gin.Context) {
	var input CreateProductWithWorkshopsInput

	// Валидация JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Неверные данные: " + err.Error(),
		})
		return
	}

	// Создание продукта с цехами в транзакции
	productID, err := CreateProductWithWorkshops(c.Request.Context(), s.pool, input)
	if err != nil {
		log.Printf("Ошибка создания продукта с цехами: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Не удалось создать продукт: " + err.Error(),
		})
		return
	}

	// Получаем созданный продукт со всеми данными
	product, err := GetProductByID(c.Request.Context(), s.pool, productID)
	if err != nil {
		log.Printf("Ошибка получения созданного продукта: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Продукт создан, но не удалось получить его данные",
		})
		return
	}

	c.JSON(http.StatusCreated, product)
}

// удаление DELEte /api/products:id
func (s *Server) DeleteById(c *gin.Context) {
	id := c.Param("id")
	var product_id int
	_, err := fmt.Sscanf(id, "%d", &product_id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "неверный id"})
		return
	}
	err = DeleteById(c.Request.Context(), s.pool, product_id)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "чет в бдшке сломалось при удалении"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "удалено успешно"})

}

// ============ MAIN ============
func main() {
	ctx := context.Background()

	// Создание пула соединений
	pool, err := pgxpool.New(ctx, "postgres://postgres:zxcqwe123@localhost:5432/practice")
	if err != nil {
		log.Fatalf("Не удалось создать пул соединений: %v", err)
	}
	defer pool.Close()

	// Проверка подключения
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}

	// Создание сервера
	server := &Server{pool: pool}

	// Настройка роутера
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	// Роуты API
	api := r.Group("/api")
	{
		api.GET("/products", server.GetProductsHandler)
		api.GET("/products/:id", server.GetProductByIDHandler)
		api.POST("/products", server.CreateProductHandler)
		api.POST("/products/with-workshops", server.CreateProductWithWorkshopsHandler)
		api.DELETE("/products/:id", server.DeleteById)

	}
	r.GET("/", server.ProductsListHandler)
	r.GET("/products/new", server.ProductsNewHandler)
	r.POST("/products/create", server.ProductsCreateHandler)
	r.POST("/products/:id/delete", server.ProductsDeleteHandler)

	// Запуск сервера
	log.Println("Сервер запущен на :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) ProductsListHandler(c *gin.Context) {
	products, err := GetAllProducts(c.Request.Context(), s.pool)
	if err != nil {
		log.Printf("Ошибка получения продуктов: %v", err)
		c.HTML(http.StatusInternalServerError, "layout.html", gin.H{
			"Title": "Ошибка",
			"Page":  "products",
		})
		return
	}

	c.HTML(http.StatusOK, "layout.html", gin.H{
		"Title":    "Список продуктов",
		"Page":     "products",
		"Products": products,
		"Message":  c.Query("message"), // для показа сообщений после создания/удаления
	})
}

// GET /products/new - форма создания
func (s *Server) ProductsNewHandler(c *gin.Context) {
	materials, _ := GetAllMaterials(c.Request.Context(), s.pool)
	types, _ := GetAllProductTypes(c.Request.Context(), s.pool)
	workshops, _ := GetAllWorkshops(c.Request.Context(), s.pool)

	templateData := gin.H{
		"Title":     "Создать продукт",
		"Page":      "products_new",
		"Materials": materials,
		"Types":     types,
		"Workshops": workshops,
	}

	c.HTML(http.StatusOK, "layout.html", templateData)
}

// POST /products/create - создание продукта
func (s *Server) ProductsCreateHandler(c *gin.Context) {
	// Получаем данные из формы
	productName := c.PostForm("product_name")
	materialID, _ := strconv.Atoi(c.PostForm("material_id"))
	typeID, _ := strconv.Atoi(c.PostForm("type_id"))
	minPrice, _ := strconv.ParseFloat(c.PostForm("min_price"), 64)
	article := c.PostForm("article")

	// Собираем цеха
	var workshops []WorkshopInput
	for key := range c.Request.PostForm {
		if len(key) > 12 && key[:12] == "workshop_id_" {
			suffix := key[12:]
			workshopID, _ := strconv.Atoi(c.PostForm("workshop_id_" + suffix))
			productionTime, _ := strconv.ParseFloat(c.PostForm("production_time_"+suffix), 64)

			if workshopID > 0 && productionTime > 0 {
				workshops = append(workshops, WorkshopInput{
					WorkshopID:     workshopID,
					ProductionTime: productionTime,
				})
			}
		}
	}

	// Создаём продукт
	input := CreateProductWithWorkshopsInput{
		ProductName: productName,
		MaterialID:  materialID,
		TypeID:      typeID,
		MinPrice:    minPrice,
		Article:     article,
		Workshops:   workshops,
	}

	_, err := CreateProductWithWorkshops(c.Request.Context(), s.pool, input)
	if err != nil {
		log.Printf("Ошибка создания продукта: %v", err)

		// Показываем форму с ошибкой
		materials, _ := GetAllMaterials(c.Request.Context(), s.pool)
		types, _ := GetAllProductTypes(c.Request.Context(), s.pool)
		workshopsData, _ := GetAllWorkshops(c.Request.Context(), s.pool)

		c.HTML(http.StatusBadRequest, "layout.html", gin.H{
			"Title":     "Создать продукт",
			"Page":      "products_new",
			"Materials": materials,
			"Types":     types,
			"Workshops": workshopsData,
			"Error":     "Не удалось создать продукт: " + err.Error(),
		})
		return
	}

	// Редирект на список с сообщением об успехе
	c.Redirect(http.StatusSeeOther, "/?message=Продукт успешно создан")
}

// POST /products/:id/delete - удаление продукта
func (s *Server) ProductsDeleteHandler(c *gin.Context) {
	id := c.Param("id")
	productID, err := strconv.Atoi(id)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}

	err = DeleteById(c.Request.Context(), s.pool, productID)
	if err != nil {
		log.Printf("Ошибка удаления продукта: %v", err)
		c.Redirect(http.StatusSeeOther, "/?error=Не удалось удалить продукт")
		return
	}

	c.Redirect(http.StatusSeeOther, "/?message=Продукт успешно удалён")
}

// Вспомогательные функции для получения справочников
type Material struct {
	ID           int    `json:"id"`
	MaterialName string `json:"material_name"`
}

type ProductType struct {
	ID       int    `json:"id"`
	TypeName string `json:"type_name"`
}

type Workshop struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetAllMaterials(ctx context.Context, pool *pgxpool.Pool) ([]Material, error) {
	query := `SELECT id, material_name FROM materials ORDER BY material_name`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materials []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.MaterialName); err != nil {
			return nil, err
		}
		materials = append(materials, m)
	}
	return materials, nil
}

func GetAllProductTypes(ctx context.Context, pool *pgxpool.Pool) ([]ProductType, error) {
	query := `SELECT id, type_name FROM products_types ORDER BY type_name`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []ProductType
	for rows.Next() {
		var t ProductType
		if err := rows.Scan(&t.ID, &t.TypeName); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, nil
}
