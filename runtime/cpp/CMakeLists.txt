cmake_minimum_required(VERSION 3.14)
project(ntt_runtime_cpp CXX)

# ntt::runtime is a header-only C++17 library. The Go-side codegen
# (backend/cpp) emits source that #includes "ntt/runtime.hpp".
add_library(ntt_runtime INTERFACE)
add_library(ntt::runtime ALIAS ntt_runtime)

target_include_directories(ntt_runtime INTERFACE
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)
target_compile_features(ntt_runtime INTERFACE cxx_std_17)
