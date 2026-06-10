-- 智慧城市地下综合管廊燃气泄漏监测系统
-- PostgreSQL 数据库初始化脚本

-- 创建数据库
CREATE DATABASE gas_monitoring
    WITH
    OWNER = postgres
    ENCODING = 'UTF8'
    LC_COLLATE = 'en_US.UTF-8'
    LC_CTYPE = 'en_US.UTF-8'
    TABLESPACE = pg_default
    CONNECTION LIMIT = -1;

\c gas_monitoring;

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS postgis_topology;

-- 检测器表
CREATE TABLE detectors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    install_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_calib TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    geom GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_detectors_device_id ON detectors(device_id);
CREATE INDEX idx_detectors_fire_zone ON detectors(fire_zone);
CREATE INDEX idx_detectors_geom ON detectors USING GIST(geom);

-- 温湿度氧气传感器表
CREATE TABLE environment_sensors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    sensor_type VARCHAR(20) NOT NULL,
    location_type VARCHAR(20) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 阀门表
CREATE TABLE valves (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    valve_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'open',
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    last_action TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 排风机表
CREATE TABLE fans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    fan_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'stopped',
    speed INTEGER DEFAULT 0,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 管廊路径表
CREATE TABLE pipe_corridor_path (
    id SERIAL PRIMARY KEY,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    segment_id INTEGER,
    geom GEOMETRY(Point, 4326)
);

CREATE INDEX idx_pipe_corridor_path_position ON pipe_corridor_path(position);
CREATE INDEX idx_pipe_corridor_path_geom ON pipe_corridor_path USING GIST(geom);

-- 防火分区表
CREATE TABLE fire_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    zone_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    start_position DOUBLE PRECISION NOT NULL,
    end_position DOUBLE PRECISION NOT NULL,
    start_latitude DOUBLE PRECISION NOT NULL,
    start_longitude DOUBLE PRECISION NOT NULL,
    end_latitude DOUBLE PRECISION NOT NULL,
    end_longitude DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 告警表
CREATE TABLE alarms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) NOT NULL,
    level INTEGER NOT NULL,
    level_name VARCHAR(20) NOT NULL,
    concentration DOUBLE PRECISION NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    message TEXT,
    timestamp TIMESTAMP NOT NULL,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_at TIMESTAMP,
    acknowledged_by VARCHAR(50),
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    resolved_note TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alarms_device_id ON alarms(device_id);
CREATE INDEX idx_alarms_level ON alarms(level);
CREATE INDEX idx_alarms_timestamp ON alarms(timestamp DESC);
CREATE INDEX idx_alarms_resolved ON alarms(resolved);

-- 阀门控制记录表
CREATE TABLE valve_control_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    valve_id VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    fire_zone VARCHAR(50),
    triggered_by VARCHAR(50) NOT NULL,
    reason TEXT,
    timestamp TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    response TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_valve_control_valve_id ON valve_control_logs(valve_id);
CREATE INDEX idx_valve_control_timestamp ON valve_control_logs(timestamp DESC);

-- 风机控制记录表
CREATE TABLE fan_control_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    fan_id VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    speed INTEGER DEFAULT 0,
    fire_zone VARCHAR(50),
    triggered_by VARCHAR(50) NOT NULL,
    reason TEXT,
    timestamp TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 泄漏源表
CREATE TABLE leak_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    leak_rate DOUBLE PRECISION NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    diffusion_radius DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    detected_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_leak_sources_status ON leak_sources(status);
CREATE INDEX idx_leak_sources_detected_at ON leak_sources(detected_at DESC);

-- 传感器健康状态表
CREATE TABLE sensor_health (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    health DOUBLE PRECISION NOT NULL,
    temperature DOUBLE PRECISION,
    voltage DOUBLE PRECISION,
    signal_strength DOUBLE PRECISION,
    last_update TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sensor_health_device_id ON sensor_health(device_id);
CREATE INDEX idx_sensor_health_last_update ON sensor_health(last_update DESC);

-- 短信发送记录表
CREATE TABLE sms_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    alarm_id UUID REFERENCES alarms(id),
    receiver VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    sent_at TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    response TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 初始化管廊路径数据 (30公里管廊，每100米一个点)
INSERT INTO pipe_corridor_path (position, latitude, longitude, segment_id, geom)
SELECT
    (n * 100.0) as position,
    39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002) as latitude,
    116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001) as longitude,
    (n / 50)::integer as segment_id,
    ST_SetSRID(ST_MakePoint(
        116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001),
        39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002)
    ), 4326) as geom
FROM generate_series(0, 300) as n;

-- 初始化检测器数据 (300台激光检测器，每100米一台)
INSERT INTO detectors (device_id, name, position, latitude, longitude, fire_zone, status, health, geom)
SELECT
    'LASER-' || LPAD(n::text, 4, '0') as device_id,
    '激光甲烷检测器-' || LPAD(n::text, 4, '0') as name,
    (n * 100.0) as position,
    39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002) as latitude,
    116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001) as longitude,
    'ZONE-' || LPAD(((n / 30) + 1)::text, 2, '0') as fire_zone,
    'normal' as status,
    100.0 as health,
    ST_SetSRID(ST_MakePoint(
        116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001),
        39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002)
    ), 4326) as geom
FROM generate_series(0, 299) as n;

-- 初始化环境传感器 (50台氧气/温湿度传感器)
INSERT INTO environment_sensors (device_id, name, sensor_type, location_type, latitude, longitude, fire_zone)
SELECT
    CASE WHEN n % 2 = 0 THEN 'ENV-O2-' || LPAD(((n/2) + 1)::text, 2, '0')
         ELSE 'ENV-TH-' || LPAD(((n/2) + 1)::text, 2, '0')
    END as device_id,
    CASE WHEN n % 2 = 0 THEN '氧气传感器-' || LPAD(((n/2) + 1)::text, 2, '0')
         ELSE '温湿度传感器-' || LPAD(((n/2) + 1)::text, 2, '0')
    END as name,
    CASE WHEN n % 2 = 0 THEN 'oxygen' ELSE 'temp_humidity' END as sensor_type,
    CASE WHEN n < 25 THEN 'vent' ELSE 'valve' END as location_type,
    39.9042 + ((n * 600) * 0.0008) as latitude,
    116.4074 + ((n * 600) * 0.001) as longitude,
    'ZONE-' || LPAD(((n * 2) + 1)::text, 2, '0') as fire_zone
FROM generate_series(0, 49) as n;

-- 初始化阀门 (50个防火分区阀门)
INSERT INTO valves (valve_id, name, fire_zone, latitude, longitude)
SELECT
    'VALVE-' || LPAD(n::text, 2, '0') as valve_id,
    '防火分区阀门-' || LPAD(n::text, 2, '0') as name,
    'ZONE-' || LPAD(n::text, 2, '0') as fire_zone,
    39.9042 + ((n * 1000) * 0.0008) as latitude,
    116.4074 + ((n * 1000) * 0.001) as longitude
FROM generate_series(1, 50) as n;

-- 初始化排风机
INSERT INTO fans (fan_id, name, fire_zone, latitude, longitude)
SELECT
    'FAN-' || LPAD(n::text, 2, '0') as fan_id,
    '排风机-' || LPAD(n::text, 2, '0') as name,
    'ZONE-' || LPAD(n::text, 2, '0') as fire_zone,
    39.9042 + ((n * 1000) * 0.0008) as latitude,
    116.4074 + ((n * 1000) * 0.001) as longitude
FROM generate_series(1, 50) as n;

-- 初始化防火分区
INSERT INTO fire_zones (zone_id, name, start_position, end_position,
    start_latitude, start_longitude, end_latitude, end_longitude)
SELECT
    'ZONE-' || LPAD(n::text, 2, '0') as zone_id,
    '防火分区' || n || '区' as name,
    (n - 1) * 600.0 as start_position,
    n * 600.0 as end_position,
    39.9042 + (((n - 1) * 600) * 0.0008) as start_latitude,
    116.4074 + (((n - 1) * 600) * 0.001) as start_longitude,
    39.9042 + (n * 600 * 0.0008) as end_latitude,
    116.4074 + (n * 600 * 0.001) as end_longitude
FROM generate_series(1, 50) as n;

-- 创建更新时间触发器函数
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_detectors_update
    BEFORE UPDATE ON detectors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_environment_sensors_update
    BEFORE UPDATE ON environment_sensors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_valves_update
    BEFORE UPDATE ON valves
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_fans_update
    BEFORE UPDATE ON fans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_alarms_unresolved ON alarms(resolved) WHERE resolved = FALSE;
CREATE INDEX idx_leak_sources_active ON leak_sources(status) WHERE status = 'active';

-- ===================== 新增功能表 =====================

-- 光纤传感器表
CREATE TABLE fiber_optic_sensors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    fiber_type VARCHAR(20) NOT NULL DEFAULT 'brillouin',
    channel_number INTEGER NOT NULL,
    start_position DOUBLE PRECISION NOT NULL,
    end_position DOUBLE PRECISION NOT NULL,
    spatial_resolution DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    install_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_calib TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_fiber_sensors_device_id ON fiber_optic_sensors(device_id);
CREATE INDEX idx_fiber_sensors_status ON fiber_optic_sensors(status);

-- 应变异常表
CREATE TABLE strain_anomalies (
    id UUID PRIMARY KEY,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    length DOUBLE PRECISION NOT NULL,
    max_strain DOUBLE PRECISION NOT NULL,
    avg_strain DOUBLE PRECISION NOT NULL,
    temperature DOUBLE PRECISION,
    confidence DOUBLE PRECISION NOT NULL,
    type VARCHAR(20) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    detected_at TIMESTAMP NOT NULL,
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    resolved_note TEXT,
    geom GEOMETRY(LineString, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_strain_anomalies_position ON strain_anomalies(position);
CREATE INDEX idx_strain_anomalies_severity ON strain_anomalies(severity);
CREATE INDEX idx_strain_anomalies_resolved ON strain_anomalies(resolved);
CREATE INDEX idx_strain_anomalies_geom ON strain_anomalies USING GIST(geom);

-- 管道表
CREATE TABLE pipes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pipe_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    diameter DOUBLE PRECISION NOT NULL,
    material VARCHAR(50) NOT NULL,
    original_wall_thickness DOUBLE PRECISION NOT NULL,
    start_position DOUBLE PRECISION NOT NULL,
    end_position DOUBLE PRECISION NOT NULL,
    start_latitude DOUBLE PRECISION NOT NULL,
    start_longitude DOUBLE PRECISION NOT NULL,
    end_latitude DOUBLE PRECISION NOT NULL,
    end_longitude DOUBLE PRECISION NOT NULL,
    install_date TIMESTAMP,
    status VARCHAR(20) DEFAULT 'normal',
    geom GEOMETRY(LineString, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pipes_pipe_id ON pipes(pipe_id);
CREATE INDEX idx_pipes_status ON pipes(status);

-- 管道腐蚀数据表
CREATE TABLE pipe_corrosion_data (
    id UUID PRIMARY KEY,
    pipe_id VARCHAR(50) NOT NULL,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    original_wall_thickness DOUBLE PRECISION NOT NULL,
    current_wall_thickness DOUBLE PRECISION NOT NULL,
    inspection_date TIMESTAMP NOT NULL,
    corrosion_rate DOUBLE PRECISION,
    predicted_rate DOUBLE PRECISION,
    remaining_life_years DOUBLE PRECISION,
    replacement_priority VARCHAR(20),
    next_inspection_date TIMESTAMP,
    geom GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_corrosion_data_pipe_id ON pipe_corrosion_data(pipe_id);
CREATE INDEX idx_corrosion_data_priority ON pipe_corrosion_data(replacement_priority);
CREATE INDEX idx_corrosion_data_inspection_date ON pipe_corrosion_data(inspection_date DESC);

-- 腐蚀预测表
CREATE TABLE corrosion_predictions (
    id UUID PRIMARY KEY,
    pipe_id VARCHAR(50) NOT NULL,
    prediction_date TIMESTAMP NOT NULL,
    model VARCHAR(20) NOT NULL,
    predicted_thickness DOUBLE PRECISION[] NOT NULL,
    time_horizon_months INTEGER[] NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_corrosion_predictions_pipe_id ON corrosion_predictions(pipe_id);
CREATE INDEX idx_corrosion_predictions_date ON corrosion_predictions(prediction_date DESC);

-- 燃气组分分析仪表
CREATE TABLE gas_analyzers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_gas_analyzers_device_id ON gas_analyzers(device_id);

-- 华白数数据表
CREATE TABLE wobbe_indices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    high_heating_value DOUBLE PRECISION NOT NULL,
    low_heating_value DOUBLE PRECISION NOT NULL,
    relative_density DOUBLE PRECISION NOT NULL,
    wobbe_index_high DOUBLE PRECISION NOT NULL,
    wobbe_index_low DOUBLE PRECISION NOT NULL,
    burning_velocity DOUBLE PRECISION,
    status VARCHAR(20) NOT NULL,
    target_wobbe DOUBLE PRECISION NOT NULL,
    deviation DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_wobbe_indices_device_id ON wobbe_indices(device_id);
CREATE INDEX idx_wobbe_indices_timestamp ON wobbe_indices(timestamp DESC);

-- 混气阀表
CREATE TABLE gas_mixing_valves (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    valve_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    current_opening DOUBLE PRECISION DEFAULT 0.0,
    target_opening DOUBLE PRECISION DEFAULT 0.0,
    min_opening DOUBLE PRECISION DEFAULT 0.0,
    max_opening DOUBLE PRECISION DEFAULT 100.0,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_gas_mixing_valves_source_type ON gas_mixing_valves(source_type);

-- 混气阀控制记录表
CREATE TABLE gas_valve_control_logs (
    id UUID PRIMARY KEY,
    valve_id VARCHAR(50) NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    opening DOUBLE PRECISION NOT NULL,
    target_opening DOUBLE PRECISION NOT NULL,
    adjustment DOUBLE PRECISION NOT NULL,
    reason TEXT,
    timestamp TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_gas_valve_control_valve_id ON gas_valve_control_logs(valve_id);
CREATE INDEX idx_gas_valve_control_timestamp ON gas_valve_control_logs(timestamp DESC);

-- 安全出口表
CREATE TABLE exit_points (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    exit_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'available',
    capacity INTEGER DEFAULT 50,
    has_stairs BOOLEAN DEFAULT TRUE,
    has_elevator BOOLEAN DEFAULT FALSE,
    geom GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_exit_points_position ON exit_points(position);
CREATE INDEX idx_exit_points_status ON exit_points(status);

-- 人员定位表
CREATE TABLE person_locations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    person_id VARCHAR(50) NOT NULL,
    name VARCHAR(100),
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    assigned_route UUID,
    timestamp TIMESTAMP NOT NULL,
    geom GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_person_locations_person_id ON person_locations(person_id);
CREATE INDEX idx_person_locations_fire_zone ON person_locations(fire_zone);
CREATE INDEX idx_person_locations_status ON person_locations(status);
CREATE INDEX idx_person_locations_timestamp ON person_locations(timestamp DESC);

-- 疏散路线表
CREATE TABLE evacuation_routes (
    id UUID PRIMARY KEY,
    alarm_id UUID REFERENCES alarms(id),
    fire_zone VARCHAR(50) NOT NULL,
    calculated_at TIMESTAMP NOT NULL,
    path JSONB NOT NULL,
    total_distance DOUBLE PRECISION NOT NULL,
    estimated_time_minutes DOUBLE PRECISION NOT NULL,
    exit_points JSONB,
    blocked_segments TEXT[],
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_evacuation_routes_alarm_id ON evacuation_routes(alarm_id);
CREATE INDEX idx_evacuation_routes_fire_zone ON evacuation_routes(fire_zone);
CREATE INDEX idx_evacuation_routes_status ON evacuation_routes(status);

-- 广播消息表
CREATE TABLE broadcast_messages (
    id UUID PRIMARY KEY,
    fire_zone VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    message_type VARCHAR(20) NOT NULL,
    priority INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    broadcasted BOOLEAN DEFAULT FALSE,
    broadcasted_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_broadcast_messages_fire_zone ON broadcast_messages(fire_zone);
CREATE INDEX idx_broadcast_messages_priority ON broadcast_messages(priority);
CREATE INDEX idx_broadcast_messages_timestamp ON broadcast_messages(timestamp DESC);

-- ===================== 新增初始化数据 =====================

-- 初始化光纤传感器 (10条光纤)
INSERT INTO fiber_optic_sensors (device_id, name, fiber_type, channel_number, start_position, end_position, spatial_resolution)
SELECT
    'FIBER-' || LPAD(n::text, 2, '0') as device_id,
    '分布式光纤传感器-' || LPAD(n::text, 2, '0') as name,
    'brillouin' as fiber_type,
    n as channel_number,
    (n - 1) * 3000.0 as start_position,
    n * 3000.0 as end_position,
    1.0 as spatial_resolution
FROM generate_series(1, 10) as n;

-- 初始化管道 (30公里管道，每1000米一段)
INSERT INTO pipes (pipe_id, name, diameter, material, original_wall_thickness,
    start_position, end_position, start_latitude, start_longitude, end_latitude, end_longitude)
SELECT
    'PIPE-' || LPAD(n::text, 3, '0') as pipe_id,
    '燃气管道-' || LPAD(n::text, 3, '0') as name,
    CASE WHEN n % 3 = 0 THEN 0.8 ELSE 0.6 END as diameter,
    CASE WHEN n % 2 = 0 THEN 'steel' ELSE 'cast_iron' END as material,
    12.0 as original_wall_thickness,
    (n - 1) * 1000.0 as start_position,
    n * 1000.0 as end_position,
    39.9042 + (((n - 1) * 1000) * 0.0008) as start_latitude,
    116.4074 + (((n - 1) * 1000) * 0.001) as start_longitude,
    39.9042 + (n * 1000 * 0.0008) as end_latitude,
    116.4074 + (n * 1000 * 0.001) as end_longitude
FROM generate_series(1, 30) as n;

-- 初始化管道腐蚀检测数据 (每段管道一个检测点)
INSERT INTO pipe_corrosion_data (id, pipe_id, position, latitude, longitude,
    original_wall_thickness, current_wall_thickness, inspection_date,
    corrosion_rate, predicted_rate, remaining_life_years, replacement_priority, next_inspection_date)
SELECT
    uuid_generate_v4() as id,
    'PIPE-' || LPAD(n::text, 3, '0') as pipe_id,
    (n * 1000.0) - 500.0 as position,
    39.9042 + (((n * 1000) - 500) * 0.0008) as latitude,
    116.4074 + (((n * 1000) - 500) * 0.001) as longitude,
    12.0 as original_wall_thickness,
    ROUND((12.0 - (random() * 4.0))::numeric, 2) as current_wall_thickness,
    CURRENT_TIMESTAMP - (random() * INTERVAL '90 days') as inspection_date,
    ROUND((random() * 0.8)::numeric, 3) as corrosion_rate,
    ROUND((random() * 0.8)::numeric, 3) as predicted_rate,
    ROUND((5.0 + random() * 15.0)::numeric, 1) as remaining_life_years,
    CASE WHEN random() < 0.1 THEN 'high'
         WHEN random() < 0.3 THEN 'medium'
         ELSE 'low' END as replacement_priority,
    CURRENT_TIMESTAMP + (90 + random() * 90) * INTERVAL '1 day' as next_inspection_date
FROM generate_series(1, 30) as n;

-- 初始化燃气组分分析仪 (3台)
INSERT INTO gas_analyzers (device_id, name, position, latitude, longitude)
SELECT
    'ANALYZER-' || LPAD(n::text, 2, '0') as device_id,
    '燃气组分分析仪-' || LPAD(n::text, 2, '0') as name,
    n * 10000.0 as position,
    39.9042 + (n * 10000 * 0.0008) as latitude,
    116.4074 + (n * 10000 * 0.001) as longitude
FROM generate_series(1, 3) as n;

-- 初始化混气阀 (3个气源)
INSERT INTO gas_mixing_valves (valve_id, name, source_type, current_opening, target_opening, position, latitude, longitude)
VALUES
    ('source-a', '甲烷气源阀', 'methane', 60.0, 60.0, 0.0, 39.9042, 116.4074),
    ('source-b', '氢气气源阀', 'hydrogen', 20.0, 20.0, 100.0, 39.9042, 116.40745),
    ('source-c', '天然气气源阀', 'natural_gas', 40.0, 40.0, 200.0, 39.9042, 116.4075);

-- 初始化安全出口 (每2公里一个出口，共16个)
INSERT INTO exit_points (exit_id, name, position, latitude, longitude, status, capacity)
SELECT
    'EXIT-' || LPAD(n::text, 2, '0') as exit_id,
    '安全出口-' || LPAD(n::text, 2, '0') as name,
    (n - 1) * 2000.0 as position,
    39.9042 + ((n - 1) * 2000 * 0.0008) as latitude,
    116.4074 + ((n - 1) * 2000 * 0.001) as longitude,
    'available' as status,
    50 + (random() * 50)::integer as capacity
FROM generate_series(1, 16) as n;

-- 新增触发器
CREATE TRIGGER trigger_fiber_sensors_update
    BEFORE UPDATE ON fiber_optic_sensors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_pipes_update
    BEFORE UPDATE ON pipes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_gas_analyzers_update
    BEFORE UPDATE ON gas_analyzers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_gas_mixing_valves_update
    BEFORE UPDATE ON gas_mixing_valves
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_exit_points_update
    BEFORE UPDATE ON exit_points
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
