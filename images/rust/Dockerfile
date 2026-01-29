# Rust agentbox image
FROM rust:1.77-slim-bookworm

# Create non-root agent user
RUN groupadd -g 1000 agent && \
    useradd -m -u 1000 -g agent -s /bin/bash agent

# Install common utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    openssh-client \
    jq \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Install common Rust tools
RUN rustup component add rustfmt clippy && \
    cargo install cargo-watch cargo-edit

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Switch to agent user
USER agent

# Configure Rust for agent user
ENV CARGO_HOME=/home/agent/.cargo
ENV RUSTUP_HOME=/home/agent/.rustup
ENV PATH="/home/agent/.cargo/bin:$PATH"

CMD ["/bin/bash"]
