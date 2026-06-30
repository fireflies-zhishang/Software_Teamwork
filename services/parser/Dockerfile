ARG IMAGE_REGISTRY_PREFIX=
ARG PYTHON_VERSION=3.12
ARG DEBIAN_VARIANT=slim
ARG UV_VERSION=0.11.6

FROM ${IMAGE_REGISTRY_PREFIX}python:${PYTHON_VERSION}-${DEBIAN_VARIANT} AS builder

WORKDIR /app

ENV PYTHONUNBUFFERED=1 \
    UV_LINK_MODE=copy \
    PATH="/app/.venv/bin:${PATH}"

ARG APT_MIRROR=
ARG APT_SECURITY_MIRROR=
ARG UV_DEFAULT_INDEX=
ARG UV_INDEX=
ARG PIP_INDEX_URL=
ARG UV_VERSION

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    if [ -n "$APT_MIRROR" ]; then \
        security_mirror="${APT_SECURITY_MIRROR:-${APT_MIRROR%/}-security}"; \
        sed -i "s|http://deb.debian.org/debian-security|$security_mirror|g; s|http://deb.debian.org/debian|$APT_MIRROR|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        libglib2.0-0 \
        libgl1 \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

RUN --mount=type=cache,target=/root/.cache/pip \
    if [ -n "$PIP_INDEX_URL" ]; then \
        python -m pip install --index-url "$PIP_INDEX_URL" uv==${UV_VERSION}; \
    else \
        python -m pip install uv==${UV_VERSION}; \
    fi

COPY pyproject.toml uv.lock README.md ./
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ -n "$UV_DEFAULT_INDEX" ]; then export UV_DEFAULT_INDEX; fi \
    && if [ -n "$UV_INDEX" ]; then export UV_INDEX; fi \
    && uv sync --frozen --no-dev --extra paddleocr --no-install-project

COPY src ./src
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ -n "$UV_DEFAULT_INDEX" ]; then export UV_DEFAULT_INDEX; fi \
    && if [ -n "$UV_INDEX" ]; then export UV_INDEX; fi \
    && uv sync --frozen --no-dev --extra paddleocr

FROM ${IMAGE_REGISTRY_PREFIX}python:${PYTHON_VERSION}-${DEBIAN_VARIANT} AS runtime

WORKDIR /app

ENV PYTHONUNBUFFERED=1 \
    UV_LINK_MODE=copy \
    XDG_CACHE_HOME=/tmp/parser-cache \
    PATH="/app/.venv/bin:${PATH}"

ARG APT_MIRROR=
ARG APT_SECURITY_MIRROR=
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    if [ -n "$APT_MIRROR" ]; then \
        security_mirror="${APT_SECURITY_MIRROR:-${APT_MIRROR%/}-security}"; \
        sed -i "s|http://deb.debian.org/debian-security|$security_mirror|g; s|http://deb.debian.org/debian|$APT_MIRROR|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        libglib2.0-0 \
        libgl1 \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app /app

RUN useradd --system --uid 10001 --create-home parser \
    && chown -R parser:parser /app /tmp/parser-cache
USER parser

EXPOSE 8080

CMD ["parser-service"]
