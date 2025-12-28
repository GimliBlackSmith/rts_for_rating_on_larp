CREATE TABLE players (
    id SERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE,
    username VARCHAR(255),
    full_name VARCHAR(255) NOT NULL,
    current_rating INTEGER NOT NULL DEFAULT 1000,
    current_level INTEGER NOT NULL DEFAULT 1 CHECK (current_level BETWEEN 1 AND 5),
    role VARCHAR(20) NOT NULL DEFAULT 'player' CHECK (role IN ('player', 'moderator', 'admin', 'super_admin')),
    ratings_available INTEGER DEFAULT 20,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_players_role ON players(role);
CREATE INDEX idx_players_level ON players(current_level);
CREATE INDEX idx_players_rating ON players(current_rating DESC);

CREATE TABLE player_links (
    id SERIAL PRIMARY KEY,
    player_id INTEGER UNIQUE NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    link_hash VARCHAR(64) UNIQUE NOT NULL,
    qr_code_path VARCHAR(500),
    access_count INTEGER DEFAULT 0,
    last_accessed TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_player_links_hash ON player_links(link_hash);
CREATE INDEX idx_player_links_player ON player_links(player_id);

CREATE TABLE game_cycles (
    id SERIAL PRIMARY KEY,
    cycle_number INTEGER NOT NULL UNIQUE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes >= 15),
    rating_timeout_minutes INTEGER NOT NULL CHECK (rating_timeout_minutes > 0),
    is_active BOOLEAN DEFAULT FALSE,
    level_recalculation_done BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_game_cycles_active ON game_cycles(is_active);
CREATE INDEX idx_game_cycles_time_range ON game_cycles(start_time, end_time);

CREATE TABLE cycle_rating_limits (
    id SERIAL PRIMARY KEY,
    game_cycle_id INTEGER NOT NULL REFERENCES game_cycles(id) ON DELETE CASCADE,
    player_level INTEGER NOT NULL CHECK (player_level BETWEEN 1 AND 5),
    ratings_per_cycle INTEGER NOT NULL CHECK (ratings_per_cycle > 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_cycle_id, player_level)
);

CREATE INDEX idx_cycle_rating_limits_cycle_level ON cycle_rating_limits(game_cycle_id, player_level);

CREATE TABLE player_ratings (
    id BIGSERIAL PRIMARY KEY,
    rater_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    rated_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    rating_type VARCHAR(10) NOT NULL CHECK (rating_type IN ('like', 'dislike')),
    rating_value INTEGER NOT NULL,
    base_value INTEGER NOT NULL CHECK (base_value IN (1, -1)),
    penalty_coefficient DECIMAL(3,2) DEFAULT 1.0 CHECK (penalty_coefficient BETWEEN 0 AND 1),
    game_cycle_id INTEGER NOT NULL REFERENCES game_cycles(id),
    calculation_details TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_player_ratings_rater_cycle ON player_ratings(rater_id, game_cycle_id);
CREATE INDEX idx_player_ratings_rated_cycle ON player_ratings(rated_id, game_cycle_id);
CREATE INDEX idx_player_ratings_cycle_time ON player_ratings(game_cycle_id, created_at);
CREATE INDEX idx_player_ratings_rater_rated_time ON player_ratings(rater_id, rated_id, created_at DESC);

CREATE TABLE rating_transfers (
    id BIGSERIAL PRIMARY KEY,
    sender_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    receiver_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    amount INTEGER NOT NULL CHECK (amount > 0),
    game_cycle_id INTEGER REFERENCES game_cycles(id),
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_rating_transfers_sender_cycle ON rating_transfers(sender_id, game_cycle_id);
CREATE INDEX idx_rating_transfers_receiver_cycle ON rating_transfers(receiver_id, game_cycle_id);
CREATE INDEX idx_rating_transfers_time ON rating_transfers(created_at);

CREATE TABLE level_boundaries (
    id SERIAL PRIMARY KEY,
    game_cycle_id INTEGER NOT NULL REFERENCES game_cycles(id) ON DELETE CASCADE,
    level_number INTEGER NOT NULL CHECK (level_number BETWEEN 1 AND 5),
    min_rating INTEGER NOT NULL,
    max_rating INTEGER NOT NULL,
    target_percentage_min INTEGER NOT NULL,
    target_percentage_max INTEGER NOT NULL,
    actual_percentage DECIMAL(5,2),
    player_count INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_cycle_id, level_number)
);

CREATE INDEX idx_level_boundaries_cycle ON level_boundaries(game_cycle_id);
CREATE INDEX idx_level_boundaries_level ON level_boundaries(level_number);

CREATE TABLE player_level_history (
    id BIGSERIAL PRIMARY KEY,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    old_level INTEGER,
    new_level INTEGER NOT NULL,
    old_rating INTEGER,
    new_rating INTEGER,
    game_cycle_id INTEGER NOT NULL REFERENCES game_cycles(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_player_level_history_player ON player_level_history(player_id, created_at DESC);
CREATE INDEX idx_player_level_history_cycle ON player_level_history(game_cycle_id);
CREATE INDEX idx_player_level_history_level ON player_level_history(new_level, created_at);

CREATE TABLE operations_log (
    id BIGSERIAL PRIMARY KEY,
    operation_type VARCHAR(50) NOT NULL CHECK (operation_type IN (
        'rating_like',
        'rating_dislike',
        'rating_transfer',
        'player_creation',
        'admin_action',
        'level_change',
        'cycle_start',
        'cycle_end',
        'system_event'
    )),
    initiator_id INTEGER REFERENCES players(id) ON DELETE SET NULL,
    initiator_level INTEGER,
    initiator_rating_before INTEGER,
    initiator_rating_after INTEGER,
    target_id INTEGER REFERENCES players(id) ON DELETE SET NULL,
    target_level INTEGER,
    target_rating_before INTEGER,
    target_rating_after INTEGER,
    game_cycle_id INTEGER REFERENCES game_cycles(id),
    rating_change INTEGER,
    rating_value INTEGER,
    details JSONB,
    ip_address INET,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_operations_log_type_time ON operations_log(operation_type, created_at DESC);
CREATE INDEX idx_operations_log_initiator_time ON operations_log(initiator_id, created_at DESC);
CREATE INDEX idx_operations_log_target_time ON operations_log(target_id, created_at DESC);
CREATE INDEX idx_operations_log_cycle_ops ON operations_log(game_cycle_id, operation_type);
CREATE INDEX idx_operations_log_rating_changes ON operations_log(created_at, rating_change);

CREATE TABLE admin_actions (
    id BIGSERIAL PRIMARY KEY,
    admin_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    action_type VARCHAR(50) NOT NULL CHECK (action_type IN (
        'create_player',
        'adjust_rating',
        'change_cycle_settings',
        'create_admin',
        'change_player_role',
        'regenerate_qr',
        'force_level_recalc',
        'set_rating_limits'
    )),
    target_player_id INTEGER REFERENCES players(id) ON DELETE SET NULL,
    details JSONB NOT NULL,
    operation_log_id BIGINT REFERENCES operations_log(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_admin_actions_admin ON admin_actions(admin_id, created_at DESC);
CREATE INDEX idx_admin_actions_action_type ON admin_actions(action_type, created_at);
CREATE INDEX idx_admin_actions_target_admin ON admin_actions(target_player_id, admin_id);

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
