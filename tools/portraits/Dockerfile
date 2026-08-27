# Portrait renderer: Godot headless + Vellichor + the minimal DAT subset.
#
# This image contains retail game data (about 454 MiB, the skeletons, faces and
# equipment models a character portrait can touch). It is NOT redistributable.
# Push it only to a private registry.
#
# Built on the workstation, because that is the only place the DAT archives
# exist. It is not built in CI and cannot be: the archives are correctly
# gitignored out of every repository.
#
# Rendering is software-only. There is no GPU in the cluster, so Mesa's lavapipe
# provides Vulkan and llvmpipe provides GL, both under Xvfb. Slower than a real
# GPU and entirely adequate: a portrait is a single frame of one character.

ARG GODOT_VERSION=4.7.1-stable
ARG TARGETARCH

# ---------------------------------------------------------------- builder
# Godot resolves the C# assembly through its own project layout, so the
# assembly is built by Godot itself rather than placed by hand. Doing it with a
# bare `dotnet build` produces a dll Godot will not load: it reports
# "Cannot instantiate C# script" because the layout under .godot/mono is what
# it actually looks at.
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build

ARG GODOT_VERSION
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl unzip \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    case "$TARGETARCH" in \
        arm64) GARCH=linux_arm64 ;; \
        amd64) GARCH=linux_x86_64 ;; \
        *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/godot.zip \
        "https://github.com/godotengine/godot-builds/releases/download/${GODOT_VERSION}/Godot_v${GODOT_VERSION}_mono_${GARCH}.zip"; \
    unzip -q /tmp/godot.zip -d /opt; \
    mv /opt/Godot_v${GODOT_VERSION}_mono_${GARCH} /opt/godot; \
    exe="$(find /opt/godot -maxdepth 1 -type f -executable -name 'Godot*' | head -1)"; \
    test -n "$exe"; ln -s "$exe" /usr/local/bin/godot; \
    rm /tmp/godot.zip

WORKDIR /src
COPY vellichor/ ./

# Import first: build-solutions needs the project to have been opened once.
# Both are allowed to fail loudly but the assembly check below is what decides.
RUN godot --headless --path /src --import 2>&1 | tail -5 || true
RUN godot --headless --path /src --build-solutions --quit 2>&1 | tail -20 || true

# Fail the build here rather than shipping an image that cannot instantiate the
# scene, which is a confusing runtime error a long way from its cause.
RUN find /src/.godot/mono -name 'Vellichor.dll' -print -quit | grep -q . \
    || { echo 'Vellichor.dll was not produced; the C# build failed' >&2; \
         find /src/.godot -maxdepth 4 -type d >&2; exit 1; }

# ---------------------------------------------------------------- runtime
# Microsoft's runtime image rather than debian:bookworm-slim, because Debian
# does not package dotnet-runtime-8.0 and Godot's .NET build needs it. This
# image is bookworm underneath, so everything below is unchanged.
FROM mcr.microsoft.com/dotnet/runtime:8.0

ARG GODOT_VERSION
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive \
    LIBGL_ALWAYS_SOFTWARE=1 \
    GALLIUM_DRIVER=llvmpipe \
    XDG_RUNTIME_DIR=/tmp/runtime \
    DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl unzip xvfb xauth \
        libgl1 libglx-mesa0 libegl1 libgles2 \
        mesa-vulkan-drivers libvulkan1 \
        libxcursor1 libxinerama1 libxrandr2 libxi6 libxkbcommon0 \
        libasound2 libpulse0 libfontconfig1 libfreetype6 libudev1 \
        python3 python3-pymysql imagemagick \
    && rm -rf /var/lib/apt/lists/*

# Godot ships per-arch archives whose inner binary is named after the arch.
RUN set -eux; \
    case "$TARGETARCH" in \
        arm64) GARCH=linux_arm64 ;; \
        amd64) GARCH=linux_x86_64 ;; \
        *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/godotengine/godot-builds/releases/download/${GODOT_VERSION}/Godot_v${GODOT_VERSION}_mono_${GARCH}.zip"; \
    curl -fsSL -o /tmp/godot.zip "$url"; \
    unzip -q /tmp/godot.zip -d /opt; \
    mv /opt/Godot_v${GODOT_VERSION}_mono_${GARCH} /opt/godot; \
    # The archive folder is named with an underscore before the arch while the
    # binary inside uses a dot, so the executable is located rather than named.
    exe="$(find /opt/godot -maxdepth 1 -type f -executable -name 'Godot*' | head -1)"; \
    test -n "$exe"; \
    ln -s "$exe" /usr/local/bin/godot; \
    godot --version; \
    rm /tmp/godot.zip

WORKDIR /app

# The whole project as Godot left it, including the .godot directory holding
# the imported assets and the built assembly.
COPY --from=build /src/ /app/vellichor/
# The DAT subset. Vellichor's InstallLocator accepts an explicit path, so the
# corpus lives outside the project directory and is mounted by configuration
# rather than discovered.
COPY corpus/ /app/corpus/
COPY render_portraits.py /app/
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod +x /usr/local/bin/entrypoint.sh && mkdir -p /tmp/runtime && chmod 700 /tmp/runtime

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
