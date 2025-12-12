-- Создание таблиц для системы управления производством

-- Таблица материалов
CREATE TABLE materials (
    id SERIAL PRIMARY KEY,
    material_name VARCHAR(255) NOT NULL,
    wasting_percentage DECIMAL(5,2)
);

-- Таблица типов продукции
CREATE TABLE products_types (
    id SERIAL PRIMARY KEY,
    type_name VARCHAR(255) NOT NULL,
    type_ratio DECIMAL(10,2)
);

-- Таблица цехов
CREATE TABLE workshops (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(100)
);

-- Таблица продуктов
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    product_name VARCHAR(255) NOT NULL,
    material_id INTEGER NOT NULL,
    type_id INTEGER NOT NULL,
    min_price DECIMAL(10,2),
    article VARCHAR(100),
    CONSTRAINT fk_products_material 
        FOREIGN KEY (material_id) 
        REFERENCES materials(id) 
        ON DELETE RESTRICT,
    CONSTRAINT fk_products_type 
        FOREIGN KEY (type_id) 
        REFERENCES products_types(id) 
        ON DELETE RESTRICT
);

-- Таблица связи продуктов и цехов
CREATE TABLE products_workshop (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL,
    workshop_id INTEGER NOT NULL,
    production_time DECIMAL(10,2),
    CONSTRAINT fk_pw_product 
        FOREIGN KEY (product_id) 
        REFERENCES products(id) 
        ON DELETE CASCADE,
    CONSTRAINT fk_pw_workshop 
        FOREIGN KEY (workshop_id) 
        REFERENCES workshops(id) 
        ON DELETE CASCADE,
    CONSTRAINT unique_product_workshop 
        UNIQUE (product_id, workshop_id)
);

-- Индексы
CREATE INDEX idx_products_material ON products(material_id);
CREATE INDEX idx_products_type ON products(type_id);
CREATE INDEX idx_pw_product ON products_workshop(product_id);
CREATE INDEX idx_pw_workshop ON products_workshop(workshop_id);
