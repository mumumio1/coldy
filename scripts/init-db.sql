-- Initialize databases for local development

-- Create databases
CREATE DATABASE IF NOT EXISTS coldy;

-- Switch to coldy database
\c coldy;

-- Create extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

