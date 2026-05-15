# Base image for agentbox containers
FROM debian:bookworm-slim

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

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Default to agent user
USER agent

# Default shell
CMD ["/bin/bash"]
