-- +goose Up
CREATE TABLE currencies (
    code   VARCHAR(3) PRIMARY KEY,
    name   VARCHAR(50) NOT NULL,
    symbol VARCHAR(5),
    is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE exchange_rates (
    base_currency  VARCHAR(3) NOT NULL,
    quote_currency VARCHAR(3) NOT NULL,
    rate           NUMERIC(15,6) NOT NULL,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (base_currency, quote_currency)
);

-- Seed common currencies
INSERT INTO currencies (code, name, symbol) VALUES
    ('USD', 'US Dollar',          '$'),
    ('EUR', 'Euro',               '€'),
    ('RUB', 'Russian Ruble',      '₽'),
    ('GBP', 'British Pound',      '£'),
    ('CNY', 'Chinese Yuan',       '¥'),
    ('JPY', 'Japanese Yen',       '¥'),
    ('CHF', 'Swiss Franc',        '₣'),
    ('CAD', 'Canadian Dollar',    'C$'),
    ('AUD', 'Australian Dollar',  'A$'),
    ('AED', 'UAE Dirham',         'د.إ'),
    ('TRY', 'Turkish Lira',       '₺'),
    ('KZT', 'Kazakhstani Tenge',  '₸'),
    ('BYN', 'Belarusian Ruble',   'Br'),
    ('GEL', 'Georgian Lari',      '₾'),
    ('AMD', 'Armenian Dram',      '֏'),
    ('UZS', 'Uzbekistani Som',    'soʻm'),
    ('UAH', 'Ukrainian Hryvnia',  '₴'),
    ('PLN', 'Polish Zloty',       'zł'),
    ('CZK', 'Czech Koruna',       'Kč'),
    ('HUF', 'Hungarian Forint',   'Ft');

-- +goose Down
DROP TABLE exchange_rates;
DROP TABLE currencies;
