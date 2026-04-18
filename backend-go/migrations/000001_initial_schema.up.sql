-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL,
    hashed_password VARCHAR(255) NOT NULL,
    company_name VARCHAR(255),
    home_country VARCHAR(3),
    default_product_categories TEXT[],
    subscription_tier VARCHAR(50) NOT NULL DEFAULT 'free',
    api_calls_remaining INTEGER NOT NULL DEFAULT 1000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ix_users_email ON users (email);

-- Product Categories
CREATE TABLE product_categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    parent_id UUID REFERENCES product_categories(id),
    slug VARCHAR(255) NOT NULL UNIQUE,
    keywords TEXT[] NOT NULL,
    hs_code VARCHAR(10),
    hs_description VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_product_categories_hs_code ON product_categories (hs_code);

-- Businesses
CREATE TABLE businesses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(500) NOT NULL,
    website VARCHAR(500),
    google_place_id VARCHAR(255),
    linkedin_url VARCHAR(500),
    country_code VARCHAR(3),
    city VARCHAR(255),
    address TEXT,
    latitude NUMERIC(10,8),
    longitude NUMERIC(11,8),
    industry VARCHAR(255),
    sub_industry VARCHAR(255),
    employee_count_range VARCHAR(50),
    estimated_revenue_range VARCHAR(50),
    business_type VARCHAR(100),
    description TEXT,
    phone VARCHAR(50),
    email VARCHAR(255),
    social_links JSONB DEFAULT '{}',
    data_source VARCHAR(100),
    enrichment_status VARCHAR(50) NOT NULL DEFAULT 'raw',
    last_enriched_at TIMESTAMPTZ,
    data_confidence NUMERIC(3,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_businesses_country_code ON businesses (country_code);
CREATE INDEX ix_businesses_google_place_id ON businesses (google_place_id);
CREATE INDEX ix_businesses_industry ON businesses (industry);

-- Trade Flows
CREATE TABLE trade_flows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    reporter_code VARCHAR(3) NOT NULL,
    partner_code VARCHAR(3) NOT NULL,
    hs_code VARCHAR(10) NOT NULL,
    year INTEGER NOT NULL,
    flow_type VARCHAR(10) NOT NULL,
    trade_value_usd INTEGER,
    net_weight_kg INTEGER,
    quantity INTEGER,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_trade_flows_hs_code ON trade_flows (hs_code);
CREATE INDEX ix_trade_flows_partner_code ON trade_flows (partner_code);
CREATE INDEX ix_trade_flows_reporter_code ON trade_flows (reporter_code);

-- Markets
CREATE TABLE markets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    country_code VARCHAR(3) NOT NULL,
    region VARCHAR(255),
    city VARCHAR(255),
    latitude NUMERIC(10,8),
    longitude NUMERIC(11,8),
    radius_km INTEGER,
    product_category_id UUID REFERENCES product_categories(id),
    estimated_market_size NUMERIC(15,2),
    saturation_score NUMERIC(3,2),
    demand_score NUMERIC(3,2),
    last_analyzed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_market_location_product UNIQUE (country_code, city, product_category_id)
);
CREATE INDEX ix_markets_country_code ON markets (country_code);

-- Business-Market junction
CREATE TABLE business_markets (
    business_id UUID NOT NULL REFERENCES businesses(id),
    market_id UUID NOT NULL REFERENCES markets(id),
    relevance_score NUMERIC(3,2),
    discovered_via VARCHAR(100),
    PRIMARY KEY (business_id, market_id)
);

-- Searches
CREATE TABLE searches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    search_type VARCHAR(50) NOT NULL,
    query JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    result_count INTEGER,
    completed_at TIMESTAMPTZ,
    task_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- AI Request Log
CREATE TABLE ai_request_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    request_type VARCHAR(100) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    model VARCHAR(100) NOT NULL,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cost_usd DOUBLE PRECISION,
    latency_ms INTEGER,
    cache_hit BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Competitors
CREATE TABLE competitors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    market_id UUID NOT NULL REFERENCES markets(id),
    name VARCHAR(500) NOT NULL,
    website VARCHAR(500),
    country_code VARCHAR(3),
    competitor_type VARCHAR(100),
    estimated_market_share NUMERIC(5,2),
    strengths TEXT[],
    weaknesses TEXT[],
    data_source VARCHAR(100),
    last_analyzed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lead Scores
CREATE TABLE lead_scores (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id UUID NOT NULL REFERENCES businesses(id),
    market_id UUID NOT NULL REFERENCES markets(id),
    user_id UUID NOT NULL REFERENCES users(id),
    overall_score INTEGER NOT NULL,
    purchase_likelihood INTEGER,
    deal_size_potential INTEGER,
    urgency_score INTEGER,
    fit_score INTEGER,
    accessibility_score INTEGER,
    scoring_rationale TEXT,
    strengths TEXT[],
    weaknesses TEXT[],
    recommended_approach TEXT,
    model_version VARCHAR(50) NOT NULL,
    prompt_version VARCHAR(50) NOT NULL,
    scored_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);

-- Market Analyses
CREATE TABLE market_analyses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    market_id UUID NOT NULL REFERENCES markets(id),
    analysis_type VARCHAR(100) NOT NULL,
    content JSONB NOT NULL,
    summary TEXT,
    generated_by VARCHAR(50) NOT NULL DEFAULT 'azure_openai',
    model_version VARCHAR(50),
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ
);

-- Market Rankings
CREATE TABLE market_rankings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    product_category_id UUID NOT NULL REFERENCES product_categories(id),
    country_code VARCHAR(3) NOT NULL,
    topsis_score NUMERIC(5,4),
    saw_score NUMERIC(5,4),
    opportunity_score NUMERIC(5,4),
    reliability_score NUMERIC(5,4),
    accessibility_score NUMERIC(5,4),
    criteria_data JSONB NOT NULL,
    algorithm_used VARCHAR(50),
    ranked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Price Snapshots
CREATE TABLE price_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    competitor_id UUID NOT NULL REFERENCES competitors(id),
    product_name VARCHAR(500),
    product_category_id UUID REFERENCES product_categories(id),
    price NUMERIC(12,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    price_unit VARCHAR(100),
    source_url TEXT,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_price_snapshots_competitor_id ON price_snapshots (competitor_id);
