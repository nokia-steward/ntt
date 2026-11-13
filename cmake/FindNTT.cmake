# BSD 3-Clause License
#
# Copyright (c) 2020, Nokia
# All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
#
# * Redistributions of source code must retain the above copyright notice, this
#   list of conditions and the following disclaimer.
#
# * Redistributions in binary form must reproduce the above copyright notice,
#   this list of conditions and the following disclaimer in the documentation
#   and/or other materials provided with the distribution.
#
# * Neither the name of the copyright holder nor the names of its
#   contributors may be used to endorse or promote products derived from
#   this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
# DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
# FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
# DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
# SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
# CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
# OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
# OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
#

#[====================================================================[.rst:
NTT Tests
--------

This module provides functions to help use the NTT/K3 Test infrastructure. It
provides function :command:`add_ttcn3_suite` for generating a test suite
manifest.

.. command:: add_ttnc3_suite

  Add a TTCN-3 test suite manifest for use with NTT and CTest::

  add_ttcn3_suite(TGT
        SOURCES src1...
        [DEPENDS ...]
        [NAME name]
        [TIMEOUT secs]
        [HOOKS_FILE executable]
        [PARAMETERS_FILE file]
        [WORKING_DIRECTORY dir]
        [TARGETS target1...]
        [DIAGNOSTICS flags]
  )

  ``add_ttcn3_suite`` creates a target TGT and test suite manifest
  ``package.yml`` as ``BYPRODUCT``.

  The options are:

  ``SOURCES src1...``
    The list of .ttcn3 source files. Files specified in SOURCES usually are
    testcases.

  ``DEPENDS target1...|directory...
    Specifies additional TTCN-3 packages required by the test suite.
    If the argument specifies a target it will be replaced by the location of
    the target directory (``$<TARGET_FILE_DIR>``). Additionally a target-level
    dependency will be added so that the depending target will be built before
    this test suite.

  ``NAME``
    Specfies the name of the test suite. If this option is not provided, ntt
    will assign one.

  ``TIMEOUT seconds``
    Specifies a timeout after which a test case will be aborted.

    ``HOOKS_FILE executable``
    Specifies an executable to be used as test hook.
    If HOOKS_FILE specifies an executable target (created by ADD_EXECUTABLE) it
    will automatically be replaced by the location of the executable created at
    build time. Additionally a target-level dependency will be added so that
    the executable target will be built before this hook is used.

  ``PARAMETERS_FILE file``
    Specifies a file containing TOML formatted test configuration.

  ``WORKING_DIRECTORY dir``
    Specifies the directory in which to run the tests. If this option is not
    provided, the current binary directory is used.

  ``TARGETS target1...``
    Add additional target-level dependencies. This is used to assure SUTs are
    built before a test is executed.

  ``DIAGNOSTICS flags``
    Specifies the compiler diagnostics flags

]====================================================================]

if(TTCN3_PROTOBUF_INCLUDED)
     return()
endif()
set(TTCN3_PROTOBUF_INCLUDED true)

find_program(NTT_EXECUTABLE NAMES ntt k3 DOC "Path to NTT")

if(NTT_EXECUTABLE)
    execute_process(
        COMMAND ${NTT_EXECUTABLE} version
        OUTPUT_VARIABLE NTT_version_output
        ERROR_VARIABLE NTT_version_error
        RESULT_VARIABLE NTT_version_result
        OUTPUT_STRIP_TRAILING_WHITESPACE
    )
    string(REGEX REPLACE "[^ ]+ ([0-9.]+).*$" "\\1" NTT_VERSION "${NTT_version_output}")

    if(NOT TARGET ntt::ntt)
        add_executable(ntt::ntt IMPORTED)
        if(EXISTS "$NTT_EXECUTABLE")
            set_property(TARGET ntt::ntt PROPERTIES IMPORTED_LOCATION "${NTT_EXECUTABLE}")
        endif()
    endif()
endif()

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(NTT
    FOUND_VAR NTT_FOUND
    REQUIRED_VARS NTT_EXECUTABLE
    VERSION_VAR NTT_VERSION
)

mark_as_advanced(NTT_EXECUTABLE)

file(REMOVE "${CMAKE_BINARY_DIR}/ttcn3_suites.json")

function(add_ttcn3_suite TGT)
    set("ARGS_PREFIX" "")
    set("ARGS_OPTIONS" "")
    set("ARGS_ONE_VALUE" "NAME;TIMEOUT;HOOKS_FILE;PARAMETERS_FILE;WORKING_DIRECTORY;DIAGNOSTICS")
    set("ARGS_MULTI_VALUE" "VARS;SOURCES;DEPENDS;TARGETS")
    cmake_parse_arguments("${ARGS_PREFIX}" "${ARGS_OPTIONS}" "${ARGS_ONE_VALUE}" "${ARGS_MULTI_VALUE}" ${ARGN})

    if (NOT _WORKING_DIRECTORY)
        set(_WORKING_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}")
    endif()
    file(MAKE_DIRECTORY "${_WORKING_DIRECTORY}")

    set(MANIFEST_FILE "${_WORKING_DIRECTORY}/package.yml")

    add_custom_target("${TGT}" DEPENDS "${MANIFEST_FILE}")
    add_custom_target("${TGT}.lint"  COMMAND NTT_CACHE=${CMAKE_BINARY_DIR} ${NTT_EXECUTABLE} lint            "${_WORKING_DIRECTORY}" >"${TGT}.lint"  DEPENDS "${TGT}")
    add_custom_target("${TGT}.tags"  COMMAND NTT_CACHE=${CMAKE_BINARY_DIR} ${NTT_EXECUTABLE} tags            "${_WORKING_DIRECTORY}" >"${TGT}.tags"  DEPENDS "${TGT}")
    add_custom_target("${TGT}.tests" COMMAND NTT_CACHE=${CMAKE_BINARY_DIR} ${NTT_EXECUTABLE} list tests      "${_WORKING_DIRECTORY}" >"${TGT}.tests" DEPENDS "${TGT}")
    add_custom_target("${TGT}.deps"  COMMAND NTT_CACHE=${CMAKE_BINARY_DIR} ${NTT_EXECUTABLE} list imports -v "${_WORKING_DIRECTORY}" >"${TGT}.deps"  DEPENDS "${TGT}")

    set(MANIFEST "")
    string(APPEND MANIFEST "# DO NOT MODIFY.\n")
    string(APPEND MANIFEST "# This file was generated by ${CMAKE_CURRENT_LIST_FILE}:${CMAKE_CURRENT_LIST_LINE}\n")

    if (_VARS)
        string(APPEND MANIFEST "variables:\n")
        foreach(var ${_VARS})
            string(APPEND MANIFEST "  ${var}: \"${${var}}\"\n")
        endforeach()
    endif()

    if (_NAME)
        string(APPEND MANIFEST "name: ${_NAME}\n")
    endif()

    if (_TIMEOUT)
        string(APPEND MANIFEST "timeout: ${_TIMEOUT}\n")
    endif()

    if (_HOOKS_FILE)
        if(TARGET "${_HOOKS_FILE}")
            string(APPEND MANIFEST "hooks_file: $<TARGET_FILE:${_HOOKS_FILE}>\n")
            add_dependencies("${TGT}" "${_HOOKS_FILE}")
        else()
            string(APPEND MANIFEST "hooks_file: ${_HOOKS_FILE}\n")
        endif()
    endif()

    if (_PARAMETERS_FILE)
        string(APPEND MANIFEST "parameters_file: ${_PARAMETERS_FILE}\n")
    endif()

    if (_SOURCES)
        string(APPEND MANIFEST "sources:\n")
        foreach(src ${_SOURCES})
            get_filename_component(abs ${src} ABSOLUTE)
            string(APPEND MANIFEST "  - ${abs}\n")
        endforeach()
    endif()

    if (_DEPENDS)
        string(APPEND MANIFEST "imports:\n")
        foreach(x ${_DEPENDS})
            if (TARGET "${x}")
                string(APPEND MANIFEST "  - $<TARGET_FILE_DIR:${x}>\n")
                add_dependencies("${TGT}" "${x}")
            else()
                get_filename_component(abs ${x} ABSOLUTE)
                if(IS_DIRECTORY "${abs}")
                    string(APPEND MANIFEST "  - ${abs}\n")
                elseif(COMMAND "${x}")
                    message(FATAL_ERROR "Command \"${x}\" does not define an existing CMake target or directory")
                else()
                    message(VERBOSE "\"${x}\" does not exist. I am assuming this is an output directory which will be created by some code-generator, later. Please make sure this directory will be created before you use this test suite.")
                    string(APPEND MANIFEST "  - ${x}\n")
                endif()
            endif()
        endforeach()
    endif()

    foreach(x ${_TARGETS})
        add_dependencies("${TGT}" "${x}")
    endforeach()

    if (_DIAGNOSTICS)
        string(REPLACE " " ";" _DIAGNOSTICS ${_DIAGNOSTICS})
        string(APPEND MANIFEST "diagnostics:\n")
        foreach(x ${_DIAGNOSTICS})
            string(APPEND MANIFEST "  - ${x}\n")
        endforeach()
    endif()

    file(GENERATE OUTPUT "${MANIFEST_FILE}" CONTENT "${MANIFEST}")

    __ntt_add_db_entry("${CMAKE_BINARY_DIR}/ttcn3_suites.json" "${TGT}" "${_WORKING_DIRECTORY}" "${CMAKE_CURRENT_LIST_DIR}")
endfunction()

function(protobuf_generate_ttcn3 TGT)
    if(NOT ARGN)
        message(SEND_ERROR "Error: protobuf_generate_ttcn3() called without any proto files")
        return()
    endif()

    if(DEFINED Protobuf_IMPORT_DIRS)
        foreach(PATH IN ITEMS ${Protobuf_IMPORT_DIRS})
            list(APPEND PROTO_PATH -I ${PATH})
        endforeach()
    endif()
    foreach(FIL ${ARGN})
        get_filename_component(ABS_FIL ${FIL} ABSOLUTE)
        get_filename_component(ABS_PATH ${ABS_FIL} PATH)
        if(NOT(ABS_PATH MATCHES ".*/itf/types" ))
            list(APPEND SRCS ${ABS_FIL})
        endif()
        list(FIND PROTO_PATH ${ABS_PATH} _CONTAINS_ALREADY)
        if(${_CONTAINS_ALREADY} EQUAL -1)
            list(APPEND PROTO_PATH -I ${ABS_PATH})
        endif()
    endforeach()

    add_custom_target(
        ${TGT}
        COMMAND ${CMAKE_COMMAND} -E make_directory ${CMAKE_CURRENT_BINARY_DIR}/ttcn3
        COMMAND ${Protobuf_PROTOC_EXECUTABLE} --ttcn3_out ${CMAKE_CURRENT_BINARY_DIR}/ttcn3 ${PROTO_PATH} ${SRCS}
        COMMENT "Running TTCN-3 protocol buffer compiler for ${TGT}"
        VERBATIM )
endfunction()

function(__ntt_add_db_entry JSON_FILE TGT ROOT_DIR SOURCE_DIR)

    # Create initial file content
    if(NOT EXISTS "${JSON_FILE}")
        file(WRITE "${JSON_FILE}" "{
  \"source_dir\": \"${CMAKE_SOURCE_DIR}\",
  \"binary_dir\": \"${CMAKE_BINARY_DIR}\",
  \"suites\": [\n]}")
    endif()

    # Remove closing brackets
    file(READ "${JSON_FILE}" CONTENTS)
    string(REGEX REPLACE "]}$" "" STRIPPED "${CONTENTS}")
    file(WRITE "${JSON_FILE}" "${STRIPPED}")

    # Append new entry
    if(STRIPPED MATCHES "}$")
        file(APPEND "${JSON_FILE}" ",\n")
    endif()
    file(APPEND "${JSON_FILE}" "    {\"root_dir\":\"${ROOT_DIR}\",\"source_dir\":\"${SOURCE_DIR}\", \"target\":\"${TGT}\"}")
    file(APPEND "${JSON_FILE}" "]}")
endfunction()
