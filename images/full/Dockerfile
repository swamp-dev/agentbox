# Full agentbox image with all languages and agents
FROM debian:bookworm-slim

# Create non-root agent user
RUN groupadd -g 1000 agent && \
    useradd -m -u 1000 -g agent -s /bin/bash agent

# Install common utilities and build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    openssh-client \
    jq \
    build-essential \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 20
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*

# Install pnpm
RUN npm install -g pnpm

# Install Python 3.12
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.22
RUN curl -fsSL https://go.dev/dl/go1.22.0.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:$PATH"

# Install Rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable
ENV PATH="/root/.cargo/bin:$PATH"

# Install AI agents
RUN npm install -g @anthropic-ai/claude-code
RUN pip3 install --no-cache-dir --break-system-packages aider-chat poetry uv

# Install Go tools
RUN go install golang.org/x/tools/gopls@latest && \
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Copy Rust and Go binaries for agent user
RUN mkdir -p /home/agent/.cargo/bin /home/agent/go/bin && \
    cp /root/.cargo/bin/* /home/agent/.cargo/bin/ 2>/dev/null || true && \
    cp /root/go/bin/* /home/agent/go/bin/ 2>/dev/null || true && \
    chown -R agent:agent /home/agent

# Switch to agent user
USER agent

# Configure environment for agent user
ENV HOME=/home/agent
ENV GOPATH=/home/agent/go
ENV CARGO_HOME=/home/agent/.cargo
ENV PATH="/home/agent/.cargo/bin:/home/agent/go/bin:/home/agent/.local/bin:/home/agent/.npm/bin:$PATH"
ENV PYTHONUNBUFFERED=1

# Configure npm for agent user
RUN mkdir -p /home/agent/.npm && \
    npm config set prefix /home/agent/.npm

CMD ["/bin/bash"]
