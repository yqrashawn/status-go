cmake_minimum_required(VERSION 3.0)
project(status-go)

SET(LIB_FILE ${CMAKE_CURRENT_SOURCE_DIR}/build/bin/libstatus${CMAKE_STATIC_LIBRARY_SUFFIX})
SET(LIB_SHARED_FILE ${CMAKE_CURRENT_SOURCE_DIR}/build/bin/libstatus${CMAKE_SHARED_LIBRARY_SUFFIX})
SET(LIB_HEADER_FOLDER ${CMAKE_CURRENT_SOURCE_DIR}/build/bin/)

if (WIN32)
	SET(BUILD_SHARED_LIB_COMMAND ${CMAKE_CURRENT_SOURCE_DIR}/scripts/build_statusgo_shared_win.bat)
	SET(BUILD_STATIC_LIB_COMMAND ${CMAKE_CURRENT_SOURCE_DIR}/scripts/build_statusgo_static_win.bat)
else()
	SET(BUILD_SHARED_LIB_COMMAND "make statusgo-shared-library")
	SET(BUILD_STATIC_LIB_COMMAND "make statusgo-library")
endif()

add_custom_command(OUTPUT  ${LIB_SHARED_FILE}
                   COMMAND ${BUILD_SHARED_LIB_COMMAND}
                   WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR})

add_custom_target(statusgo_shared_target DEPENDS ${LIB_SHARED_FILE} ${CMAKE_CURRENT_SOURCE_DIR})

add_library(statusgo_shared SHARED IMPORTED GLOBAL)

add_dependencies(statusgo_shared statusgo_shared_target)

if (WIN32)
	set_target_properties(statusgo_shared
		PROPERTIES
		IMPORTED_LOCATION ${LIB_SHARED_FILE}
		INTERFACE_INCLUDE_DIRECTORIES ${LIB_HEADER_FOLDER}
		IMPORTED_IMPLIB ${LIB_FILE}) # we must add this for Windows https://cmake.org/cmake/help/v3.23/command/add_library.html?highlight=imported_implib#imported-libraries
else()
	set_target_properties(statusgo_shared
		PROPERTIES
		IMPORTED_LOCATION ${LIB_SHARED_FILE}
		INTERFACE_INCLUDE_DIRECTORIES ${LIB_HEADER_FOLDER})
endif()


add_custom_command(OUTPUT  ${LIB_FILE}
                   COMMAND ${BUILD_STATIC_LIB_COMMAND}
                   WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR})

add_custom_target(statusgo_target DEPENDS ${LIB_FILE})

add_library(statusgo STATIC IMPORTED GLOBAL)

add_dependencies(statusgo statusgo_target)

set_target_properties(statusgo
    PROPERTIES
    IMPORTED_LOCATION ${LIB_FILE}
    INTERFACE_INCLUDE_DIRECTORIES ${LIB_HEADER_FOLDER})
