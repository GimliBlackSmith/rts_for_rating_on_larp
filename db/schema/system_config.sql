CREATE TABLE system_config (
    id SERIAL PRIMARY KEY,
    rating_formula_a NUMERIC(8,4) NOT NULL,
    rating_formula_b NUMERIC(8,4) NOT NULL,
    default_cycle_duration_minutes INTEGER NOT NULL CHECK (default_cycle_duration_minutes >= 15),
    default_rating_timeout_minutes INTEGER NOT NULL CHECK (default_rating_timeout_minutes > 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE system_rating_limits (
    id SERIAL PRIMARY KEY,
    player_level INTEGER NOT NULL CHECK (player_level BETWEEN 1 AND 5),
    ratings_per_cycle INTEGER NOT NULL CHECK (ratings_per_cycle > 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(player_level)
);
