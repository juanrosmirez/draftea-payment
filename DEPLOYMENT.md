# Guía de Instalación y Deployment

## Requisitos del Sistema

### Requisitos Mínimos
- **CPU**: 2 cores
- **RAM**: 4GB
- **Almacenamiento**: 20GB
- **Sistema Operativo**: Linux, macOS, Windows

### Software Requerido
- **Go**: 1.21 o superior
- **Docker**: 20.10 o superior
- **Docker Compose**: 2.0 o superior
- **PostgreSQL**: 15 o superior (si no usa Docker)
- **Redis**: 7 o superior (si no usa Docker)
- **Apache Kafka**: 2.8 o superior (si no usa Docker)

## Instalación Rápida con Docker

### 1. Clonar el Repositorio
```bash
git clone <repository-url>
cd payment-system
```

### 2. Configurar Variables de Entorno
```bash
cp .env.example .env
# Editar .env con sus configuraciones
```

### 3. Levantar los Servicios
```bash
# Levantar toda la infraestructura
make up

# O usando docker-compose directamente
docker-compose up -d
```

### 4. Verificar el Estado
```bash
# Ver logs
make logs

# Verificar salud del servicio
curl http://localhost:8080/health
```

## Instalación Manual

### 1. Instalar Dependencias

#### PostgreSQL
```bash
# Ubuntu/Debian
sudo apt-get install postgresql postgresql-contrib

# macOS
brew install postgresql

# Crear base de datos
createdb payment_system
```

#### Redis
```bash
# Ubuntu/Debian
sudo apt-get install redis-server

# macOS
brew install redis
```

#### Apache Kafka
```bash
# Descargar y configurar Kafka
wget https://downloads.apache.org/kafka/2.8.2/kafka_2.13-2.8.2.tgz
tar -xzf kafka_2.13-2.8.2.tgz
cd kafka_2.13-2.8.2

# Iniciar Zookeeper
bin/zookeeper-server-start.sh config/zookeeper.properties

# Iniciar Kafka
bin/kafka-server-start.sh config/server.properties
```

### 2. Configurar Base de Datos
```bash
# Ejecutar migraciones
psql -U postgres -d payment_system -f migrations/001_create_tables.sql
```

### 3. Configurar Variables de Entorno
```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=payment_system
export DB_USER=postgres
export DB_PASSWORD=your_password
export REDIS_HOST=localhost
export REDIS_PORT=6379
export KAFKA_BROKERS=localhost:9092
```

### 4. Compilar y Ejecutar
```bash
# Instalar dependencias
make deps

# Compilar
make build

# Ejecutar
./bin/payment-system
```

## Configuración

### Variables de Entorno

| Variable | Descripción | Valor por Defecto |
|----------|-------------|-------------------|
| `SERVER_PORT` | Puerto del servidor HTTP | `8080` |
| `SERVER_HOST` | Host del servidor | `0.0.0.0` |
| `DB_HOST` | Host de PostgreSQL | `localhost` |
| `DB_PORT` | Puerto de PostgreSQL | `5432` |
| `DB_NAME` | Nombre de la base de datos | `payment_system` |
| `DB_USER` | Usuario de la base de datos | `postgres` |
| `DB_PASSWORD` | Contraseña de la base de datos | `` |
| `REDIS_HOST` | Host de Redis | `localhost` |
| `REDIS_PORT` | Puerto de Redis | `6379` |
| `KAFKA_BROKERS` | Brokers de Kafka | `localhost:9092` |
| `KAFKA_TOPIC` | Tópico de eventos | `payment-events` |
| `GATEWAY_PROVIDER` | Proveedor de pasarela | `stripe` |
| `GATEWAY_API_KEY` | API Key de la pasarela | `` |
| `METRICS_ENABLED` | Habilitar métricas | `true` |
| `METRICS_PORT` | Puerto de métricas | `9091` |
| `LOG_LEVEL` | Nivel de logging | `info` |

### Archivo .env de Ejemplo
```env
# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Database
DB_HOST=postgres
DB_PORT=5432
DB_NAME=payment_system
DB_USER=postgres
DB_PASSWORD=postgres123
DB_SSL_MODE=disable

# Redis
REDIS_HOST=redis
REDIS_PORT=6379

# Kafka
KAFKA_BROKERS=kafka:29092
KAFKA_TOPIC=payment-events
KAFKA_CONSUMER_GROUP=payment-service

# Gateway
GATEWAY_PROVIDER=stripe
GATEWAY_BASE_URL=https://api.stripe.com
GATEWAY_API_KEY=sk_test_your_stripe_key

# Metrics
METRICS_ENABLED=true
METRICS_PORT=9091

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

## Deployment en Producción

### 1. Preparación del Entorno

#### Configuración de Seguridad
```bash
# Crear usuario no-root
sudo useradd -m -s /bin/bash payment-system
sudo usermod -aG docker payment-system

# Configurar firewall
sudo ufw allow 8080/tcp
sudo ufw allow 9091/tcp
```

#### Configuración de Base de Datos
```sql
-- Crear usuario específico para la aplicación
CREATE USER payment_app WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE payment_system TO payment_app;

-- Configurar conexiones SSL
ALTER SYSTEM SET ssl = on;
SELECT pg_reload_conf();
```

### 2. Deployment con Docker Swarm

#### Inicializar Swarm
```bash
docker swarm init
```

#### Crear Stack File
```yaml
# docker-stack.yml
version: '3.8'

services:
  payment-service:
    image: payment-system:latest
    deploy:
      replicas: 3
      update_config:
        parallelism: 1
        delay: 10s
      restart_policy:
        condition: on-failure
    ports:
      - "8080:8080"
      - "9091:9091"
    environment:
      - DB_HOST=postgres
      - REDIS_HOST=redis
      - KAFKA_BROKERS=kafka:29092
    networks:
      - payment-network

networks:
  payment-network:
    driver: overlay
```

#### Deploy Stack
```bash
docker stack deploy -c docker-stack.yml payment-stack
```

### 3. Deployment con Kubernetes

#### Crear Namespace
```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: payment-system
```

#### ConfigMap
```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: payment-config
  namespace: payment-system
data:
  DB_HOST: "postgres-service"
  REDIS_HOST: "redis-service"
  KAFKA_BROKERS: "kafka-service:9092"
  METRICS_ENABLED: "true"
```

#### Deployment
```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-service
  namespace: payment-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: payment-service
  template:
    metadata:
      labels:
        app: payment-service
    spec:
      containers:
      - name: payment-service
        image: payment-system:latest
        ports:
        - containerPort: 8080
        - containerPort: 9091
        envFrom:
        - configMapRef:
            name: payment-config
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

#### Service
```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: payment-service
  namespace: payment-system
spec:
  selector:
    app: payment-service
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: metrics
    port: 9091
    targetPort: 9091
  type: LoadBalancer
```

#### Aplicar Configuraciones
```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

## Monitoreo y Observabilidad

### 1. Métricas con Prometheus
```yaml
# prometheus-config.yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'payment-service'
    static_configs:
      - targets: ['payment-service:9091']
    scrape_interval: 5s
```

### 2. Dashboards con Grafana
- Importar dashboards predefinidos desde `grafana/dashboards/`
- Configurar alertas para métricas críticas
- Crear dashboards personalizados según necesidades

### 3. Logging Centralizado
```yaml
# filebeat.yml
filebeat.inputs:
- type: container
  paths:
    - '/var/lib/docker/containers/*/*.log'
  processors:
  - add_docker_metadata: ~

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

## Backup y Recuperación

### 1. Backup de Base de Datos
```bash
# Backup automático diario
#!/bin/bash
BACKUP_DIR="/backups/postgres"
DATE=$(date +%Y%m%d_%H%M%S)

pg_dump -h postgres -U postgres payment_system > "$BACKUP_DIR/payment_system_$DATE.sql"

# Mantener solo los últimos 7 días
find $BACKUP_DIR -name "*.sql" -mtime +7 -delete
```

### 2. Backup de Kafka Topics
```bash
# Exportar configuración de topics
kafka-topics.sh --bootstrap-server kafka:9092 --list > topics_backup.txt

# Backup de datos (usando Kafka Connect)
# Configurar conectores para exportar a almacenamiento externo
```

### 3. Restauración
```bash
# Restaurar base de datos
psql -h postgres -U postgres -d payment_system < backup_file.sql

# Recrear topics de Kafka
while read topic; do
  kafka-topics.sh --bootstrap-server kafka:9092 --create --topic $topic --partitions 3 --replication-factor 1
done < topics_backup.txt
```

## Troubleshooting

### Problemas Comunes

#### 1. Servicio no inicia
```bash
# Verificar logs
docker-compose logs payment-service

# Verificar conectividad a dependencias
docker-compose exec payment-service ping postgres
docker-compose exec payment-service ping redis
```

#### 2. Alta latencia en pagos
```bash
# Verificar métricas de base de datos
docker-compose exec postgres psql -U postgres -c "SELECT * FROM pg_stat_activity;"

# Verificar estado del circuit breaker
curl http://localhost:8080/api/v1/metrics?metric=circuit_breaker_state
```

#### 3. Problemas de memoria
```bash
# Verificar uso de memoria
docker stats

# Ajustar límites en docker-compose.yml
services:
  payment-service:
    deploy:
      resources:
        limits:
          memory: 1G
```

### Comandos Útiles

```bash
# Ver estado de todos los servicios
make logs

# Reiniciar servicios
make restart

# Acceder a la base de datos
make db-shell

# Ejecutar tests
make test

# Ver métricas
curl http://localhost:9091/metrics

# Verificar salud
curl http://localhost:8080/health
```

## Escalabilidad

### Escalado Horizontal
```bash
# Docker Compose
docker-compose up --scale payment-service=3

# Kubernetes
kubectl scale deployment payment-service --replicas=5
```

### Optimizaciones de Rendimiento
1. **Connection Pooling**: Configurar pools de conexión adecuados
2. **Caching**: Implementar cache Redis para consultas frecuentes
3. **Particionamiento**: Particionar tablas grandes por fecha
4. **Índices**: Crear índices optimizados para consultas frecuentes
5. **Load Balancing**: Usar nginx o HAProxy para balanceo de carga

### Monitoreo de Rendimiento
```bash
# Métricas de aplicación
curl http://localhost:9091/metrics | grep payment_

# Métricas de sistema
docker stats
htop
iotop
```

## Seguridad

### 1. Configuración SSL/TLS
```nginx
# nginx.conf
server {
    listen 443 ssl;
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://payment-service:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 2. Autenticación y Autorización
- Implementar JWT tokens
- Configurar rate limiting
- Usar API keys para servicios externos

### 3. Secrets Management
```bash
# Usar Docker Secrets
echo "db_password" | docker secret create db_password -

# O usar herramientas como HashiCorp Vault
vault kv put secret/payment-system db_password="secure_password"
```

## Actualizaciones

### Rolling Updates
```bash
# Docker Swarm
docker service update --image payment-system:v2.0.0 payment-stack_payment-service

# Kubernetes
kubectl set image deployment/payment-service payment-service=payment-system:v2.0.0
```

### Blue-Green Deployment
1. Desplegar nueva versión en entorno "green"
2. Ejecutar tests de smoke
3. Cambiar tráfico de "blue" a "green"
4. Monitorear métricas y logs
5. Rollback si es necesario
