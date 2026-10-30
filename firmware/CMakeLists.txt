# doormutex traffic light firmware (ESP-IDF, ESP32-C6).
#   idf.py set-target esp32c6 build
cmake_minimum_required(VERSION 3.16)

# Shown in HA, Improv and the logs; release builds override it from the tag.
if(NOT DEFINED PROJECT_VER)
    set(PROJECT_VER "0.1.0-dev")
endif()

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(doormutex)
