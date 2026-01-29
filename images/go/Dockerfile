# Go agentbox image
FROM golang:1.22-bookworm

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
    && rm -rf /var/lib/apt/lists/*

# Install common Go tools
RUN go install golang.org/x/tools/gopls@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Switch to agent user
USER agent

# Configure Go for agent user
ENV GOPATH=/home/agent/go
ENV PATH="/home/agent/go/bin:$PATH"

CMD ["/bin/bash"]
