cmake_minimum_required(VERSION 3.21)
project(UtauTTSQt VERSION 0.1 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 6.5 REQUIRED COMPONENTS Quick QuickControls2 Multimedia Concurrent)
qt_standard_project_setup(REQUIRES 6.5)
qt_policy(SET QTP0004 NEW)

set(UTAUTTS_NATIVE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../build/native" CACHE PATH "Directory containing utautts_native shared library")

qt_add_executable(utautts
    src/main.cpp
    src/backend.cpp
    src/backend.h
    src/utautts_abi.h
)

if(WIN32)
    set_target_properties(utautts PROPERTIES WIN32_EXECUTABLE TRUE)
endif()

qt_add_qml_module(utautts
    URI UtauTTS
    VERSION 1.0
    QML_FILES
        qml/Main.qml
        qml/PitchEditor.qml
)

set_source_files_properties(
    "${CMAKE_CURRENT_SOURCE_DIR}/../LICENSE"
    PROPERTIES QT_RESOURCE_ALIAS "LICENSE"
)
set_source_files_properties(
    "${CMAKE_CURRENT_SOURCE_DIR}/../THIRD_PARTY_NOTICES.txt"
    PROPERTIES QT_RESOURCE_ALIAS "THIRD_PARTY_NOTICES.txt"
)
qt_add_resources(utautts "legal"
    PREFIX "/legal"
    FILES
        "${CMAKE_CURRENT_SOURCE_DIR}/../LICENSE"
        "${CMAKE_CURRENT_SOURCE_DIR}/../THIRD_PARTY_NOTICES.txt"
)

if(WIN32)
    target_link_libraries(utautts PRIVATE "${UTAUTTS_NATIVE_DIR}/utautts_native.dll.a")
else()
    target_link_libraries(utautts PRIVATE "${UTAUTTS_NATIVE_DIR}/libutautts_native.so")
endif()

target_include_directories(utautts PRIVATE src)
target_compile_definitions(utautts PRIVATE UTAUTTS_VERSION="${PROJECT_VERSION}")
target_link_libraries(utautts PRIVATE Qt6::Quick Qt6::QuickControls2 Qt6::Multimedia Qt6::Concurrent)

install(TARGETS utautts RUNTIME DESTINATION . BUNDLE DESTINATION .)
