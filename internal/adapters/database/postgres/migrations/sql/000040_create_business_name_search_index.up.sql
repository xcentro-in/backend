-- Create index on business name for efficient "starts-with" text searches
-- Supports ILIKE 'term%' queries used in autocomplete/search functionality
CREATE INDEX idx_business_name_search ON xcentro.business (name text_pattern_ops);
