#!/bin/bash

# start-minio-docker.sh - Start Minio as Docker container accessible to multiple clusters
# This script runs Minio in Docker for backup/restore testing in the workshop

set -e  # Exit on any error

# Configuration
CONTAINER_NAME="workshop-minio"
MINIO_PORT="9000"
MINIO_CONSOLE_PORT="9001"
MINIO_ROOT_USER="minioadmin"
MINIO_ROOT_PASSWORD="minioadmin123"
MINIO_DATA_DIR="$HOME/.workshop-minio-data"
DOCKER_NETWORK="kind"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Check if Docker is running
check_docker_running() {
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker first."
        exit 1
    fi
    log_info "Docker is running"
}

# Check if container already exists
check_existing_container() {
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_warn "Container '$CONTAINER_NAME' already exists"
        
        # Check if it's running
        if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
            log_info "Container is already running"
            show_connection_info
            return 0
        else
            log_info "Container exists but is not running"
            read -p "Do you want to start the existing container? (Y/n): " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Nn]$ ]]; then
                log_info "Removing existing container..."
                docker rm "$CONTAINER_NAME"
                return 1  # Need to create new container
            else
                log_info "Starting existing container..."
                docker start "$CONTAINER_NAME"
                wait_for_minio
                show_connection_info
                return 0
            fi
        fi
    fi
    return 1  # Container doesn't exist
}

# Create data directory
create_data_directory() {
    if [ ! -d "$MINIO_DATA_DIR" ]; then
        log_info "Creating Minio data directory: $MINIO_DATA_DIR"
        mkdir -p "$MINIO_DATA_DIR"
    else
        log_info "Using existing data directory: $MINIO_DATA_DIR"
    fi
}

# Check if kind network exists, create if needed
setup_docker_network() {
    if ! docker network ls --format '{{.Name}}' | grep -q "^${DOCKER_NETWORK}$"; then
        log_info "Creating Docker network: $DOCKER_NETWORK"
        docker network create "$DOCKER_NETWORK"
    else
        log_info "Docker network '$DOCKER_NETWORK' already exists"
    fi
}

# Start Minio container
start_minio_container() {
    log_step "Starting Minio container..."
    
    local docker_cmd=(
        docker run -d
        --name "$CONTAINER_NAME"
        --network "$DOCKER_NETWORK"
        -p "${MINIO_PORT}:9000"
        -p "${MINIO_CONSOLE_PORT}:9001"
        -v "${MINIO_DATA_DIR}:/data"
        -e "MINIO_ROOT_USER=${MINIO_ROOT_USER}"
        -e "MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}"
        --restart unless-stopped
        minio/minio:latest
        server /data --console-address ":9001"
    )
    
    log_info "Running: ${docker_cmd[*]}"
    
    if "${docker_cmd[@]}"; then
        log_info "Minio container started successfully"
    else
        log_error "Failed to start Minio container"
        exit 1
    fi
}

# Wait for Minio to be ready
wait_for_minio() {
    log_info "Waiting for Minio to be ready..."
    
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "http://localhost:${MINIO_PORT}/minio/health/live" &> /dev/null; then
            log_info "Minio is ready!"
            break
        fi
        
        log_info "Waiting for Minio to start... (attempt $attempt/$max_attempts)"
        sleep 2
        ((attempt++))
    done
    
    if [ $attempt -gt $max_attempts ]; then
        log_error "Minio failed to become ready within expected time"
        log_info "Container logs:"
        docker logs "$CONTAINER_NAME" --tail 20
        exit 1
    fi
}

# Create initial buckets for workshop
create_workshop_buckets() {
    log_step "Creating workshop buckets..."
    
    # Install mc (Minio client) in a temporary container if not available
    local mc_cmd="docker run --rm --network $DOCKER_NETWORK minio/mc:latest"
    
    # Configure mc to connect to our Minio instance
    log_info "Configuring Minio client..."
    $mc_cmd alias set workshop "http://${CONTAINER_NAME}:9000" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
    
    # Create buckets
    local buckets=("backups" "restore-test" "workshop-data")
    
    for bucket in "${buckets[@]}"; do
        log_info "Creating bucket: $bucket"
        if $mc_cmd mb "workshop/$bucket" 2>/dev/null || true; then
            log_info "✓ Bucket '$bucket' created or already exists"
        else
            log_warn "Failed to create bucket '$bucket'"
        fi
    done
    
    # List buckets to verify
    log_info "Available buckets:"
    $mc_cmd ls workshop/
}

# Show connection information
show_connection_info() {
    log_step "Minio Connection Information"
    echo ""
    echo "🚀 Minio is running and accessible!"
    echo ""
    echo "📊 Web Console:"
    echo "   URL: http://localhost:${MINIO_CONSOLE_PORT}"
    echo "   Username: ${MINIO_ROOT_USER}"
    echo "   Password: ${MINIO_ROOT_PASSWORD}"
    echo ""
    echo "🔗 API Endpoint:"
    echo "   URL: http://localhost:${MINIO_PORT}"
    echo "   From kind clusters: http://${CONTAINER_NAME}:9000"
    echo ""
    echo "🗂️  Data Directory: ${MINIO_DATA_DIR}"
    echo "🐳 Container Name: ${CONTAINER_NAME}"
    echo "🌐 Docker Network: ${DOCKER_NETWORK}"
    echo ""
    echo "📝 Workshop Buckets:"
    echo "   - backups (for storing Kubernetes resource backups)"
    echo "   - restore-test (for testing restore operations)"
    echo "   - workshop-data (for general workshop data)"
    echo ""
    echo "🛠️  Management Commands:"
    echo "   Stop:    docker stop ${CONTAINER_NAME}"
    echo "   Start:   docker start ${CONTAINER_NAME}"
    echo "   Remove:  docker rm -f ${CONTAINER_NAME}"
    echo "   Logs:    docker logs ${CONTAINER_NAME}"
    echo ""
}

# Health check function
health_check() {
    log_step "Performing health check..."
    
    # Check container status
    if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_error "Container is not running"
        return 1
    fi
    
    # Check API endpoint
    if curl -s "http://localhost:${MINIO_PORT}/minio/health/live" &> /dev/null; then
        log_info "✓ API endpoint is healthy"
    else
        log_error "✗ API endpoint is not responding"
        return 1
    fi
    
    # Check console endpoint
    if curl -s "http://localhost:${MINIO_CONSOLE_PORT}" &> /dev/null; then
        log_info "✓ Console endpoint is healthy"
    else
        log_warn "⚠ Console endpoint is not responding (this might be normal)"
    fi
    
    log_info "Health check completed successfully"
    return 0
}

# Stop and remove container
stop_minio() {
    log_step "Stopping Minio container..."
    
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        docker stop "$CONTAINER_NAME"
        log_info "Container stopped"
    fi
    
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        docker rm "$CONTAINER_NAME"
        log_info "Container removed"
    fi
    
    log_info "Minio container stopped and removed"
    log_info "Data directory preserved at: $MINIO_DATA_DIR"
}

# Show usage information
show_usage() {
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo "Start and manage Minio Docker container for workshop"
    echo ""
    echo "Commands:"
    echo "  start     Start Minio container (default)"
    echo "  stop      Stop and remove Minio container"
    echo "  status    Show container status and connection info"
    echo "  health    Perform health check"
    echo "  restart   Restart Minio container"
    echo ""
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  --port                  Minio API port (default: $MINIO_PORT)"
    echo "  --console-port          Minio console port (default: $MINIO_CONSOLE_PORT)"
    echo "  --user                  Minio root user (default: $MINIO_ROOT_USER)"
    echo "  --password              Minio root password (default: $MINIO_ROOT_PASSWORD)"
    echo "  --data-dir              Data directory (default: $MINIO_DATA_DIR)"
    echo "  --container-name        Container name (default: $CONTAINER_NAME)"
    echo ""
    echo "Examples:"
    echo "  $0                      # Start Minio with default settings"
    echo "  $0 start --port 9090    # Start with custom API port"
    echo "  $0 stop                 # Stop and remove container"
    echo "  $0 status               # Show current status"
}

# Parse command line arguments
parse_args() {
    local command="start"
    
    # Check if first argument is a command
    if [[ $# -gt 0 ]] && [[ "$1" =~ ^(start|stop|status|health|restart)$ ]]; then
        command="$1"
        shift
    fi
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            --port)
                MINIO_PORT="$2"
                shift 2
                ;;
            --console-port)
                MINIO_CONSOLE_PORT="$2"
                shift 2
                ;;
            --user)
                MINIO_ROOT_USER="$2"
                shift 2
                ;;
            --password)
                MINIO_ROOT_PASSWORD="$2"
                shift 2
                ;;
            --data-dir)
                MINIO_DATA_DIR="$2"
                shift 2
                ;;
            --container-name)
                CONTAINER_NAME="$2"
                shift 2
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    echo "$command"
}

# Main execution
main() {
    # Handle help first
    for arg in "$@"; do
        if [[ "$arg" == "-h" || "$arg" == "--help" ]]; then
            show_usage
            exit 0
        fi
    done
    
    local command
    command=$(parse_args "$@")
    
    case $command in
        start)
            log_info "Starting Minio Docker container for workshop..."
            check_docker_running
            
            if check_existing_container; then
                exit 0  # Container already running
            fi
            
            create_data_directory
            setup_docker_network
            start_minio_container
            wait_for_minio
            create_workshop_buckets
            show_connection_info
            ;;
        stop)
            stop_minio
            ;;
        status)
            if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
                log_info "Container '$CONTAINER_NAME' is running"
                show_connection_info
            else
                log_warn "Container '$CONTAINER_NAME' is not running"
                exit 1
            fi
            ;;
        health)
            health_check
            ;;
        restart)
            log_info "Restarting Minio container..."
            stop_minio
            sleep 2
            main start
            ;;
        *)
            log_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi