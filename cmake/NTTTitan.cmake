# BSD 3-Clause License
#
# Copyright (c) 2026, Nokia
# All rights reserved.

#[====================================================================[.rst:
NTTTitan
--------

Additional CMake macros for the ntt-Titan toolchain. This module
supplements `FindNTT.cmake` with helpers for the M4+ command surface
(`ntt exec`, `ntt check`, `ntt mctr`, `ntt hc`) and for migrating
existing Titan projects via `ntt migrate`.

.. command:: ntt_add_test_suite

  Add a CTest-integrated ntt test suite::

    ntt_add_test_suite(TGT
        SOURCES src1...
        [CONFIG cfg]
        [FORMAT junit|tap|json|html|text]
        [PATTERN glob...]
        [WORKING_DIRECTORY dir]
    )

  The macro creates a CTest test named ``TGT`` that runs ``ntt exec``
  on the given sources, with optional ``--cfg``, ``--format`` and
  ``--pattern`` flags forwarded directly to the binary. The output is
  written to ``${WORKING_DIRECTORY}/ntt-report.<FORMAT>`` so CI
  pipelines can pick it up as a build artefact.

.. command:: ntt_migrate_from_tpd

  One-shot migration from a Titan TPD to a ntt ``package.yml``::

    ntt_migrate_from_tpd(<tpd_file> <output_dir>)

  Generates ``${output_dir}/package.yml`` at configure time. Suitable
  for projects that want to add an ntt build alongside an existing
  Titan one without committing the converted manifest to source
  control.

]====================================================================]

if(NTT_TITAN_INCLUDED)
    return()
endif()
set(NTT_TITAN_INCLUDED true)

include(FindNTT)

function(ntt_add_test_suite TGT)
    set(_options)
    set(_one_value CONFIG FORMAT WORKING_DIRECTORY)
    set(_multi_value SOURCES PATTERN)
    cmake_parse_arguments(_NTT "${_options}" "${_one_value}" "${_multi_value}" ${ARGN})

    if(NOT _NTT_WORKING_DIRECTORY)
        set(_NTT_WORKING_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}")
    endif()
    if(NOT _NTT_FORMAT)
        set(_NTT_FORMAT "text")
    endif()

    set(_cmd ${NTT_EXECUTABLE} exec --format ${_NTT_FORMAT}
             --out ${_NTT_WORKING_DIRECTORY})
    if(_NTT_CONFIG)
        list(APPEND _cmd --cfg ${_NTT_CONFIG})
    endif()
    foreach(p IN LISTS _NTT_PATTERN)
        list(APPEND _cmd --pattern ${p})
    endforeach()
    list(APPEND _cmd ${_NTT_SOURCES})

    add_test(NAME ${TGT}
        COMMAND ${_cmd}
        WORKING_DIRECTORY ${_NTT_WORKING_DIRECTORY})
endfunction()

function(ntt_migrate_from_tpd TPD_FILE OUTPUT_DIR)
    if(NOT NTT_EXECUTABLE)
        message(FATAL_ERROR "ntt_migrate_from_tpd: NTT_EXECUTABLE not set; run find_package(NTT) first")
    endif()
    file(MAKE_DIRECTORY ${OUTPUT_DIR})
    execute_process(
        COMMAND ${NTT_EXECUTABLE} migrate from-titan ${TPD_FILE}
        OUTPUT_FILE ${OUTPUT_DIR}/package.yml
        RESULT_VARIABLE _result
    )
    if(NOT _result EQUAL 0)
        message(FATAL_ERROR "ntt migrate from-titan failed (exit ${_result})")
    endif()
endfunction()
