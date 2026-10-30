# Host unit tests for the firmware's pure logic (components/dm_core) –
# plain C with gcc/clang, no ESP-IDF needed:
#
#   cmake -S firmware/test/host -B build/host && cmake --build build/host && ctest --test-dir build/host
cmake_minimum_required(VERSION 3.16)
project(doormutex_firmware_host_tests C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)

include(FetchContent)
# cJSON, the same library and version ESP-IDF v5.5 ships. Only the sources
# are used (SOURCE_SUBDIR points nowhere, so its own project isn't added).
FetchContent_Declare(cjson
    URL https://github.com/DaveGamble/cJSON/archive/refs/tags/v1.7.19.tar.gz
    URL_HASH SHA256=7fa616e3046edfa7a28a32d5f9eacfd23f92900fe1f8ccd988c1662f30454562
    DOWNLOAD_EXTRACT_TIMESTAMP TRUE
    SOURCE_SUBDIR none
)
FetchContent_MakeAvailable(cjson)

set(CORE ${CMAKE_CURRENT_SOURCE_DIR}/../../components/dm_core)
add_executable(dm_tests
    main.c
    test_config.c
    test_discovery.c
    test_improv.c
    test_pattern.c
    test_state.c
    test_time.c
    ${CORE}/src/dm_config.c
    ${CORE}/src/dm_discovery.c
    ${CORE}/src/dm_improv.c
    ${CORE}/src/dm_pattern.c
    ${CORE}/src/dm_state.c
    ${CORE}/src/dm_time.c
    ${cjson_SOURCE_DIR}/cJSON.c
)
target_include_directories(dm_tests PRIVATE ${CORE}/include ${cjson_SOURCE_DIR})
target_compile_options(dm_tests PRIVATE -Wall -Wextra -Werror -fsanitize=address,undefined -fno-omit-frame-pointer)
target_link_options(dm_tests PRIVATE -fsanitize=address,undefined)
set_source_files_properties(${cjson_SOURCE_DIR}/cJSON.c PROPERTIES COMPILE_OPTIONS "-w")
target_link_libraries(dm_tests PRIVATE m)

enable_testing()
add_test(NAME dm_tests COMMAND dm_tests)
