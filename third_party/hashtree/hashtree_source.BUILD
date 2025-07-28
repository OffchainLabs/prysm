# BUILD file for building hashtree from source with proper cross-compilation support
load("@io_bazel_rules_go//go:def.bzl", "go_library")

package(default_visibility = ["//visibility:public"])

# Build hashtree library for AMD64 - only build when targeting x86_64
genrule(
    name = "build_hashtree_amd64",
    srcs = [
        "src/hashtree.c",
        "src/hashtree.h",
        "src/sha256_generic.c", 
        "src/sha256_shani.S",
        "src/sha256_avx_x16.S",
        "src/sha256_avx_x8.S", 
        "src/sha256_avx_x4.S",
        "src/sha256_avx_x1.S",
        "src/sha256_sse_x1.S",
    ],
    outs = ["hashtree_amd64.syso"],
    cmd = """
        # Create build directories
        mkdir -p build/obj build/lib
        
        # Use Bazel's toolchain - CC and AR are set by the toolchain
        COMPILER="$${CC:-gcc}"
        ARCHIVER="$${AR:-ar}"
        
        # Add debugging output
        echo "Building hashtree with compiler: $$COMPILER"
        echo "Building hashtree with archiver: $$ARCHIVER"
        echo "Target architecture: $${GOARCH:-unknown}"
        
        # Compile assembly files (handles Intel syntax)
        $$COMPILER -g -fpic -c $(location src/sha256_shani.S) -o build/obj/sha256_shani.o
        $$COMPILER -g -fpic -c $(location src/sha256_avx_x16.S) -o build/obj/sha256_avx_x16.o
        $$COMPILER -g -fpic -c $(location src/sha256_avx_x8.S) -o build/obj/sha256_avx_x8.o
        $$COMPILER -g -fpic -c $(location src/sha256_avx_x4.S) -o build/obj/sha256_avx_x4.o
        $$COMPILER -g -fpic -c $(location src/sha256_avx_x1.S) -o build/obj/sha256_avx_x1.o
        $$COMPILER -g -fpic -c $(location src/sha256_sse_x1.S) -o build/obj/sha256_sse_x1.o
        
        # Compile C files
        $$COMPILER -g -Wall -Werror -O3 -c $(location src/sha256_generic.c) -o build/obj/sha256_generic.o
        $$COMPILER -g -Wall -Werror -O3 -c $(location src/hashtree.c) -I. -o build/obj/hashtree.o
        
        # Create static library
        $$ARCHIVER rcs build/lib/libhashtree.a build/obj/*.o
        
        # Copy to syso file
        cp build/lib/libhashtree.a $@
    """,
    target_compatible_with = ["@platforms//cpu:x86_64"],
    tags = ["requires-network"],
)

# Build hashtree library for ARM64 - only build when targeting ARM64
genrule(
    name = "build_hashtree_arm64",
    srcs = [
        "src/hashtree.c",
        "src/hashtree.h",
        "src/sha256_generic.c",
        "src/sha256_armv8_neon_x4.S",
        "src/sha256_armv8_neon_x1.S", 
        "src/sha256_armv8_crypto.S",
    ],
    outs = ["hashtree_arm64.syso"],
    cmd = """
        # Create build directories
        mkdir -p build/obj build/lib
        
        # Try to use proper cross-compilation compiler
        if command -v clang >/dev/null 2>&1; then
            # CI environment with clang - use ARM64 cross-compilation
            COMPILER="clang --target=aarch64-linux-gnu"
            ARCHIVER="llvm-ar"
            echo "Using clang cross-compiler for ARM64"
        elif command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then
            # Local environment with ARM64 GCC cross-compiler
            COMPILER="aarch64-linux-gnu-gcc"
            ARCHIVER="aarch64-linux-gnu-ar"
            echo "Using GCC cross-compiler for ARM64"
        else
            # Fallback: use system compiler (will fail for cross-compilation)
            COMPILER="gcc"
            ARCHIVER="ar"
            echo "WARNING: Using system compiler - cross-compilation may fail"
        fi
        
        echo "Building hashtree with compiler: $$COMPILER"
        echo "Building hashtree with archiver: $$ARCHIVER"
        
        # Compile assembly files with ARM64 target
        $$COMPILER -g -fpic -c $(location src/sha256_armv8_neon_x4.S) -o build/obj/sha256_armv8_neon_x4.o || echo "Failed to compile neon_x4"
        $$COMPILER -g -fpic -c $(location src/sha256_armv8_neon_x1.S) -o build/obj/sha256_armv8_neon_x1.o || echo "Failed to compile neon_x1"
        $$COMPILER -g -fpic -c $(location src/sha256_armv8_crypto.S) -o build/obj/sha256_armv8_crypto.o || echo "Failed to compile crypto"
        
        # Compile C files with ARM64 target
        $$COMPILER -g -Wall -Werror -O3 -c $(location src/sha256_generic.c) -o build/obj/sha256_generic.o || echo "Failed to compile generic"
        $$COMPILER -g -Wall -Werror -O3 -c $(location src/hashtree.c) -I. -o build/obj/hashtree.o || echo "Failed to compile hashtree"
        
        # Create static library
        $$ARCHIVER rcs build/lib/libhashtree.a build/obj/*.o || echo "Failed to create archive"
        
        # Copy to syso file
        cp build/lib/libhashtree.a $@
    """,
    target_compatible_with = ["@platforms//cpu:aarch64"],
    tags = ["requires-network"],
)

# Empty syso file for platforms where hashtree is not available
genrule(
    name = "build_hashtree_generic",
    outs = ["hashtree_generic.syso"],
    cmd = "touch $@",
    tags = ["manual"],
)

# Go library with architecture-specific syso files
go_library(
    name = "hashtree",
    srcs = [
        "bindings.go",
        "bindings_amd64.go", 
        "bindings_arm64.go",
        "sha256_1_generic.go",
        "wrapper_linux_amd64.s",
        "wrapper_arm64.s",
    ] + select({
        "@platforms//cpu:x86_64": [":build_hashtree_amd64"],
        "@platforms//cpu:aarch64": [":build_hashtree_arm64"],
        "//conditions:default": [":build_hashtree_generic"],
    }),
    cgo = False,
    importpath = "github.com/prysmaticlabs/hashtree",
    visibility = ["//visibility:public"],
    deps = ["@com_github_klauspost_cpuid_v2//:go_default_library"],
)