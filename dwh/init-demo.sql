CREATE TABLE IF NOT EXISTS crm_users (
    id SERIAL PRIMARY KEY,
    login VARCHAR(255) NOT NULL UNIQUE,
    device_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO crm_users (login, device_id) VALUES 
('prothetic1', 'PROSTH-2024-001'),
('prothetic2', 'PROSTH-2024-002'),
('prothetic3', 'PROSTH-2024-003')
ON CONFLICT (login) DO NOTHING;

DROP TABLE IF EXISTS telemetry_raw;

CREATE TABLE telemetry_raw (
    id SERIAL PRIMARY KEY,
    device_id VARCHAR(50) NOT NULL,
    event_time TIMESTAMP NOT NULL,
    emg_signal_mean FLOAT NOT NULL,
    command_recognized VARCHAR(30),
    confidence_score FLOAT NOT NULL,
    battery_level_pct INT NOT NULL,
    battery_temp_c FLOAT NOT NULL,
    movement_detected BOOLEAN NOT NULL,
    error_code VARCHAR(20)
);

CREATE INDEX idx_telemetry_device_time ON telemetry_raw(device_id, event_time);

INSERT INTO telemetry_raw (device_id, event_time, emg_signal_mean,
                           command_recognized, confidence_score,
                           battery_level_pct, battery_temp_c,
                           movement_detected, error_code)
SELECT 
    u.device_id,
    NOW() - (n * INTERVAL '1 hour'),
    ROUND((RANDOM() * 4 + 1)::numeric, 2),
    CASE 
        WHEN RANDOM() < 0.6 THEN 'GRASP'
        WHEN RANDOM() < 0.3 THEN 'ROTATE'
        ELSE 'NONE' 
    END,
    ROUND((RANDOM() * 0.4 + 0.5)::numeric, 2),
    (90 - (n % 20)),
    ROUND((30 + (RANDOM() * 10))::numeric, 1),
    CASE WHEN RANDOM() < 0.7 THEN TRUE ELSE FALSE END,
    CASE 
        WHEN RANDOM() < 0.03 THEN 'BAT_OVERHEAT'
        WHEN RANDOM() < 0.02 THEN 'EMG_NOISE'
        ELSE NULL 
    END
FROM crm_users u
CROSS JOIN generate_series(1, 150) AS n
ON CONFLICT DO NOTHING;
