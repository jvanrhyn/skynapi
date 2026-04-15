# skynapi Architecture

A production-grade Go REST API providing city search and weather forecasting services with intelligent caching and fuzzy text search capabilities.

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Layered Architecture](#layered-architecture)
4. [Component Interactions](#component-interactions)
5. [Request Flows](#request-flows)
6. [Data Models](#data-models)
7. [Deployment Architecture](#deployment-architecture)
8. [Technology Stack](#technology-stack)

---

## Overview

**skynapi** is a two-service REST API built with Go 1.21+, providing:

- **City Search**: Fuzzy text search across 150k+ cities using PostgreSQL trigram similarity
- **Weather API**: Intelligent caching layer over met.no, with conditional GET and stale-cache fallback

**Key characteristics:**
- Clean hexagonal (layered) architecture with dependency injection
- Interface-driven design for testability
- Structured JSON logging with `slog`
- Type-safe PostgreSQL with `pgxpool`
- Graceful degradation and resilience patterns
- YAML configuration with environment overrides

---

## System Architecture

### High-Level System Diagram

```mermaid
graph TB
    Client["🌐 HTTP Client"]
    API["API Server<br/>chi router"]
    CityHandler["City Handler<br/>GET /v1/cities"]
    WeatherHandler["Weather Handler<br/>GET /v1/weather"]
    
    CityService["City Service<br/>Business Logic"]
    WeatherService["Weather Service<br/>Cache Strategy"]
    
    CityRepo["City Repository<br/>pgxpool"]
    WeatherRepo["Weather Repository<br/>pgxpool"]
    METClient["MET Client<br/>HTTP Client"]
    
    PostgreSQL["🐘 PostgreSQL<br/>all_countries + weather_cache"]
    METApi["☁️ api.met.no<br/>Locationforecast API"]
    
    Client -->|HTTP Request| API
    API -->|Route| CityHandler
    API -->|Route| WeatherHandler
    
    CityHandler -->|validate + call| CityService
    WeatherHandler -->|validate + call| WeatherService
    
    CityService -->|query| CityRepo
    WeatherService -->|get/set| WeatherRepo
    WeatherService -->|fetch| METClient
    
    CityRepo -->|SQL| PostgreSQL
    WeatherRepo -->|SQL| PostgreSQL
    METClient -->|HTTPS| METApi
    
    PostgreSQL -.->|response| WeatherRepo
    METApi -.->|JSON + headers| METClient
    
    CityRepo -.->|results| CityService
    WeatherRepo -.->|cached data| WeatherService
    METClient -.->|parsed data| WeatherService
    
    CityService -.->|200 OK| CityHandler
    WeatherService -.->|200/304/503| WeatherHandler
    
    CityHandler -.->|JSON| Client
    WeatherHandler -.->|JSON| Client
```

---

## Layered Architecture

The application follows a **clean hexagonal (onion) architecture** with four distinct layers:

```mermaid
graph TB
    subgraph HTTP["HTTP Layer<br/>(External Interface)"]
        H1["City Handler<br/>Parse query params<br/>Validate bounds<br/>Return 200/422"]
        H2["Weather Handler<br/>Parse coordinates<br/>Validate ranges<br/>Return 200/304/503"]
        HZ["Health Check<br/>GET /healthz"]
    end
    
    subgraph Service["Service Layer<br/>(Business Logic)"]
        S1["City Service<br/>Apply defaults<br/>Validate input<br/>Wrap errors"]
        S2["Weather Service<br/>Cache strategy<br/>Coordinate repo+client<br/>Conditional fetch"]
    end
    
    subgraph Data["Data Layer<br/>(Persistence & External APIs)"]
        R1["City Repository<br/>PostgreSQL<br/>Trigram search"]
        R2["Weather Repository<br/>PostgreSQL<br/>Cache store"]
        C["MET Client<br/>HTTP Client<br/>Conditional GET"]
    end
    
    subgraph External["External Dependencies"]
        DB["🐘 PostgreSQL"]
        API["☁️ api.met.no"]
    end
    
    H1 --> S1
    H2 --> S2
    HZ -->|version info| External
    
    S1 --> R1
    S2 --> R2
    S2 --> C
    
    R1 --> DB
    R2 --> DB
    C --> API
    
    style HTTP fill:#e1f5ff
    style Service fill:#fff3e0
    style Data fill:#f3e5f5
    style External fill:#eeeeee
```

### Layer Responsibilities

| Layer | Responsibility | Key Classes |
|-------|-----------------|------------|
| **HTTP** | Parse/validate requests, serialize responses | `CityHandler`, `WeatherHandler` |
| **Service** | Business logic, validation, orchestration | `CityService`, `WeatherService` |
| **Data** | Persistence, external API calls | `*Repository`, `*Client` |
| **External** | Third-party systems | PostgreSQL, api.met.no |

---

## Component Interactions

### Dependency Injection & Interface Pattern

```mermaid
graph LR
    Config["⚙️ Configuration<br/>config.yaml<br/>+ env vars"]
    Pool["pgxpool.Pool<br/>Connection manager"]
    
    CityRepo["Repository"]
    CityService["Service"]
    CityHandler["Handler"]
    
    WeatherRepo["Repository"]
    WeatherService["Service"]
    WeatherClient["Client"]
    WeatherHandler["Handler"]
    
    Config -->|parsed| Pool
    Config -->|user-agent| WeatherClient
    
    Pool -->|implements| CityRepo
    CityRepo -->|injected| CityService
    CityService -->|injected| CityHandler
    
    Pool -->|implements| WeatherRepo
    WeatherRepo -->|injected| WeatherService
    WeatherClient -->|injected| WeatherService
    WeatherService -->|injected| WeatherHandler
    
    style Config fill:#fff9c4
    style Pool fill:#c8e6c9
    style CityRepo fill:#bbdefb
    style WeatherRepo fill:#bbdefb
    style CityService fill:#ffe0b2
    style WeatherService fill:#ffe0b2
    style WeatherClient fill:#f8bbd0
```

**Key Principle:** Every component is initialized through constructors that accept interfaces, enabling:
- Easy mocking in unit tests
- Loose coupling between layers
- Testability without real databases

---

## Request Flows

### City Search Request Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as CityHandler
    participant Service as CityService
    participant Repo as CityRepository
    participant DB as PostgreSQL
    
    Client->>Handler: GET /v1/cities?q=oslo&page=1&limit=20
    Handler->>Handler: Parse query params
    Handler->>Handler: Bind to SearchParams struct
    
    Handler->>Service: SearchCities(ctx, SearchParams{Q: "oslo", Page: 1, Limit: 20})
    
    Service->>Service: Validate params<br/>(q required, 1-100 limit)
    Service->>Repo: Search(ctx, params)
    
    Repo->>DB: Execute query with<br/>trigram operators (%)
    Note over DB: WHERE name % $1<br/>OR asciiname % $1<br/>OR name ILIKE...
    
    DB-->>Repo: []City + total_count
    Repo-->>Service: results, total_count, nil
    
    Service->>Service: Build SearchResult<br/>with pagination metadata
    Service-->>Handler: SearchResult{Cities: [...], Total: 42}
    
    Handler->>Handler: Marshal to JSON
    Handler-->>Client: 200 OK<br/>{"cities": [...], "total": 42, ...}
```

### Weather Fetch Request Flow (Cache Strategy)

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WeatherHandler
    participant Service as WeatherService
    participant Repo as WeatherRepository
    participant DB as PostgreSQL
    participant METClient
    participant MET as api.met.no
    
    Client->>Handler: GET /v1/weather?lat=59.91&lon=10.75
    Handler->>Handler: Parse, validate coordinates
    
    Handler->>Service: GetWeather(ctx, 59.91, 10.75)
    Service->>Service: Normalize coords to 4 decimals
    
    alt Cache Hit & Fresh (TTL not expired)
        Service->>Repo: Get(ctx, 59.91, 10.75)
        Repo->>DB: SELECT * FROM weather_cache<br/>WHERE expires_at > NOW()
        DB-->>Repo: CachedWeather{Data, ExpiresAt}
        Repo-->>Service: cached_data, nil
        Service-->>Handler: FetchResult{Data: ..., FromCache: true}
    else Cache Hit & Stale (TTL expired)
        Service->>Repo: Get(ctx, 59.91, 10.75)
        Repo->>DB: SELECT * FROM weather_cache
        DB-->>Repo: CachedWeather (stale)
        Repo-->>Service: cached_data, nil
        Service->>METClient: Fetch(ctx, lat, lon, If-Modified-Since)
        METClient->>MET: GET /weatherapi/locationforecast/2.0/compact<br/>If-Modified-Since: Wed, 21 Oct 2025 07:28:00 GMT
        
        alt 200 OK (Data changed)
            MET-->>METClient: 200 + JSON + Expires + Last-Modified
            METClient-->>Service: FetchResult{200, new_data}
            Service->>Repo: Set(ctx, CachedWeather{new_data, expires_at})
            Repo->>DB: INSERT/UPDATE weather_cache
            Service-->>Handler: FetchResult{Data: new_data, StatusCode: 200}
        else 304 Not Modified (Data unchanged)
            MET-->>METClient: 304 + Expires + Last-Modified
            METClient-->>Service: FetchResult{304, nil}
            Service->>Repo: Set(ctx, CachedWeather{...extend TTL...})
            Repo->>DB: UPDATE expires_at = NOW() + interval
            Service-->>Handler: FetchResult{Data: stale_data, StatusCode: 304}
        end
    else Cache Miss
        Service->>Repo: Get(ctx, 59.91, 10.75)
        Repo->>DB: SELECT * (no data found)
        DB-->>Repo: nil
        Repo-->>Service: nil, nil
        Service->>METClient: Fetch(ctx, lat, lon)
        
        alt 200 OK
            METClient->>MET: GET /weatherapi/locationforecast/2.0/compact
            MET-->>METClient: 200 + JSON
            METClient-->>Service: FetchResult{200, data}
            Service->>Repo: Set(ctx, CachedWeather{data, expires_at})
            Repo->>DB: INSERT weather_cache
            Service-->>Handler: FetchResult{Data: data, StatusCode: 200}
        else Error (500/429/network)
            METClient-->>Service: FetchResult{error} or error
            Service->>Repo: Get(ctx) check stale fallback
            Repo-->>Service: nil (no cache)
            Service-->>Handler: error{503 Service Unavailable}
        end
    end
    
    Handler->>Handler: Write response
    Handler-->>Client: 200/304/503 + JSON/error
```

---

## Data Models

### Entity Relationship Diagram

```mermaid
erDiagram
    CITY ||--o{ WEATHER_CACHE : "coordinate reference"
    
    CITY {
        int geonameid PK
        string name
        string country_code
        string admin1_code
        float latitude
        float longitude
        string timezone
    }
    
    WEATHER_CACHE {
        int id PK
        float latitude
        float longitude
        json raw_response
        timestamp expires_at
        timestamp last_modified
        timestamp cached_at
    }
```

### City Model

```go
type City struct {
    GeonameID   int     `json:"geonameId"`
    Name        string  `json:"name"`
    CountryCode string  `json:"countryCode"`
    Region      string  `json:"region"`
    Lat         float64 `json:"lat"`
    Lon         float64 `json:"lon"`
    Timezone    string  `json:"timezone"`
}

type SearchParams struct {
    Q     string `validate:"required,min=1"`
    Page  int    `validate:"min=1"`
    Limit int    `validate:"min=1,max=100"`
}
```

### Weather Model

```go
type CachedWeather struct {
    Latitude      float64
    Longitude     float64
    RawResponse   json.RawMessage  // Stored JSON from api.met.no
    ExpiresAt     time.Time
    LastModified  time.Time
}

type WeatherRequest struct {
    Lat float64 `validate:"min=-90,max=90"`
    Lon float64 `validate:"min=-180,max=180"`
}
```

---

## Deployment Architecture

### Container & Infrastructure

```mermaid
graph TB
    subgraph Client["Client Devices"]
        B1["🌐 Web Browser"]
        B2["📱 Mobile App"]
        B3["🔌 API Client"]
    end
    
    subgraph Network["Network Layer"]
        LB["Load Balancer<br/>optional"]
        DNS["DNS<br/>skynapi.example.com"]
    end
    
    subgraph Docker["Docker Compose / Kubernetes"]
        subgraph skynapi["skynapi Container<br/>:8080"]
            API["Go API Server<br/>chi router"]
            Logger["slog JSON Logger<br/>stdout"]
        end
        
        subgraph postgres["PostgreSQL 15+ Container<br/>:5432"]
            pg["Database Instance"]
            pgTrgm["pg_trgm Extension"]
            pgIndices["GIN Indices"]
        end
    end
    
    subgraph External["External Services"]
        MET["☁️ api.met.no<br/>Weather Forecasts"]
    end
    
    subgraph Observability["Observability Stack<br/>optional"]
        Logs["Log Aggregator<br/>ELK/Loki"]
        Monitor["Monitoring<br/>Prometheus/Grafana"]
    end
    
    B1 -->|HTTP/HTTPS| DNS
    B2 -->|HTTP/HTTPS| DNS
    B3 -->|HTTP/HTTPS| DNS
    DNS -->|routes| LB
    LB -->|proxy| API
    
    API -->|queries| pg
    API -->|HTTPS| MET
    
    Logger -->|stdout| Logs
    Monitor -.->|scrape metrics| API
    
    style skynapi fill:#bbdefb
    style postgres fill:#c8e6c9
    style External fill:#ffccbc
    style Observability fill:#f0f4c3
```

### Environment Configuration

```mermaid
graph LR
    ConfigFile["config.yaml<br/>defaults"]
    EnvVars["Environment Variables<br/>overrides"]
    ConfigMerge["Config Merger<br/>env takes precedence"]
    Runtime["Runtime Config<br/>active values"]
    
    ConfigFile -->|merge| ConfigMerge
    EnvVars -->|override| ConfigMerge
    ConfigMerge -->|produce| Runtime
    
    Runtime --> ServerCfg["server:<br/>port, timeouts"]
    Runtime --> DBCfg["db:<br/>connection url"]
    Runtime --> METCfg["met:<br/>user-agent, base-url"]
    Runtime --> LogCfg["log:<br/>level, format"]
```

---

## Technology Stack

### Backend & Database

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| **Language** | Go | 1.21+ | Type-safe, concurrent, performant |
| **Web Framework** | chi | v5 | Lightweight HTTP router |
| **Database** | PostgreSQL | 15+ | Relational data + extensions |
| **DB Driver** | jackc/pgx | v5 | Type-safe, high-performance |
| **Connection Pool** | pgxpool | - | Safe concurrent access |
| **Logging** | slog | built-in (1.21+) | Structured JSON logging |
| **Validation** | go-playground/validator | - | Input validation |
| **HTTP Client** | net/http | built-in | External API calls |

### PostgreSQL Extensions

```mermaid
graph TD
    PG["PostgreSQL"]
    TRGM["pg_trgm<br/>Trigram similarity<br/>Fuzzy text search<br/>GIN indices"]
    JSON["json/jsonb<br/>Store raw API responses<br/>Query capabilities"]
    
    PG -->|extension| TRGM
    PG -->|extension| JSON
    
    TRGM -->|powers| CitySearch["City search with typos"]
    JSON -->|powers| WeatherCache["Raw response storage"]
```

### Development Tools

| Tool | Purpose |
|------|---------|
| **Docker** | Containerization & local development |
| **docker-compose** | Multi-container orchestration |
| **Makefile** | Build automation, version injection |
| **sqlc** | SQL-first type safety (optional) |
| **go test** | Unit & integration testing |
| **moq/testify** | Interface mocking for tests |

### Deployment Options

```mermaid
graph TB
    Code["Source Code"]
    
    Build["Build Stage<br/>go build -ldflags<br/>Version injection"]
    
    subgraph Local["Local Development"]
        DC["docker-compose up<br/>PostgreSQL + API"]
    end
    
    subgraph Cloud["Cloud Deployment"]
        K8S["Kubernetes Cluster<br/>Pod + Service"]
        K8SDB["Managed PostgreSQL<br/>AWS RDS / GCP Cloud SQL"]
    end
    
    subgraph Serverless["Serverless Options"]
        CloudRun["Cloud Run / Lambda<br/>Stateless functions"]
        HerokuDB["Heroku PostgreSQL"]
    end
    
    Code -->|docker build| Build
    Build -->|docker-compose| Local
    Build -->|kubectl apply| K8S
    K8S -->|connects| K8SDB
    Build -->|deploy| CloudRun
    CloudRun -->|connects| HerokuDB
```

---

## Key Architectural Decisions

### 1. **Interface-Driven Design**
- Every layer uses interfaces for dependencies
- Enables unit testing without real databases/APIs
- Allows easy swapping of implementations

### 2. **Cache-First Weather Strategy**
- Reduces api.met.no API calls (rate limit protection)
- Conditional GET support (304 Not Modified)
- Graceful degradation with stale-cache fallback

### 3. **Structured Logging**
- Go 1.21's `slog` for JSON output
- Production-ready for log aggregation
- Request tracing and observability

### 4. **Type-Safe PostgreSQL**
- `pgxpool` for connection safety
- No ORM (raw SQL + scanning)
- Explicit type casting for trigram operators

### 5. **Clean Separation of Concerns**
- HTTP Layer: Request parsing & response formatting only
- Service Layer: Business logic & orchestration
- Data Layer: Persistence & external APIs
- Testable via mocks at each boundary

---

## Future Enhancements

```mermaid
graph LR
    Current["Current State<br/>City search<br/>Weather caching"]
    
    Phase1["Phase 1: Observability<br/>OpenTelemetry<br/>Prometheus metrics"]
    Phase2["Phase 2: Advanced Caching<br/>Redis layer<br/>Multi-region cache"]
    Phase3["Phase 3: Real-time<br/>WebSocket support<br/>Weather alerts"]
    Phase4["Phase 4: Scale<br/>Event streaming<br/>Async processing"]
    
    Current --> Phase1
    Phase1 --> Phase2
    Phase2 --> Phase3
    Phase3 --> Phase4
```

---

## Related Documentation

- [API Specification](../api/openapi.yaml) - OpenAPI 3.0 spec
- [Configuration](../config.yaml.example) - Environment setup
- [Migrations](../migrations/) - Database schema
- [Testing Strategy](./testing.md) - TDD & test patterns

