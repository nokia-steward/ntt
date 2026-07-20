# Current Conformance Misses

Generated from `/tmp/claude-1000/-home-rdiniz-programs-ntt/2fb45c04-13fc-4156-b00a-12bcd47feab7/scratchpad/strict_default.json` using the current workspace runner.

- Summary: `ran 4948 files: 4798 matched, 23 skipped (97.42% match rate)`
- Misses: `150`

## Misses By Outcome

- `reject->pass`: `107`
- `->`: `23`
- `pass->fail`: `12`
- `error->pass`: `2`
- `fail->pass`: `2`
- `pass->parse-error`: `2`
- `error->timeout`: `1`
- `none->pass`: `1`

## Misses By Chapter

- `core_language/06_types_and_values`: `36` (`->`=9, `pass->fail`=2, `reject->pass`=25)
- `core_language/22_communication_operations`: `22` (`reject->pass`=22)
- `core_language/05_basic_language_elements`: `20` (`->`=3, `reject->pass`=17)
- `core_language/15_templates`: `18` (`->`=2, `pass->fail`=2, `reject->pass`=14)
- `core_language/16_functions_altsteps_testcases`: `10` (`->`=1, `pass->fail`=4, `reject->pass`=5)
- `core_language/20_statement_and_operations_for_alt`: `9` (`pass->fail`=1, `reject->pass`=8)
- `core_language/26_module_control`: `9` (`->`=3, `error->pass`=2, `error->timeout`=1, `fail->pass`=2, `pass->fail`=1)
- `core_language/09_test_configurations`: `7` (`reject->pass`=7)
- `core_language/08_modules`: `3` (`->`=2, `pass->fail`=1)
- `core_language/B_matching_incoming_values`: `3` (`reject->pass`=3)
- `oo/501_classes_and_objects`: `3` (`pass->fail`=1, `pass->parse-error`=2)
- `core_language/07_expressions`: `2` (`reject->pass`=2)
- `core_language/11_variables`: `2` (`->`=2)
- `core_language/21_configuration_operations`: `2` (`reject->pass`=2)
- `core_language/27_specifying_attributes`: `2` (`->`=1, `reject->pass`=1)
- `core_language/23_timer_operations`: `1` (`none->pass`=1)
- `core_language/D_preprocessing_macros`: `1` (`reject->pass`=1)

## Top Chapter Details

### `core_language/06_types_and_values`

- `core_language/06_types_and_values/0601_basic_types_and_values/060100_simple_basic_types_and_values/NegSyn_060100_SimpleBasicTypes_005.ttcn`: ` -> `
- `core_language/06_types_and_values/0601_basic_types_and_values/060100_simple_basic_types_and_values/NegSyn_060100_SimpleBasicTypes_006.ttcn`: ` -> `
- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010202_lists_of_types/NegSem_06010202_ListOfTypes_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010205_pattern_subtyping_of_character_string_types/NegSyn_06010205_StringPattern_002.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020101_referencing_fields_of_record_type/NegSem_06020101_ReferencingRecordFields_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/060201_toplevel/NegSem_060201_RecordTypeValues_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_002.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_002.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060204_enumerated_type_and_values/NegSyn_060204_enumerated_type_and_values_001.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060204_enumerated_type_and_values/NegSyn_060204_enumerated_type_and_values_002.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020501_referencing_fields_of_union_type/NegSem_06020501_referencing_fields_of_union_type_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020502_option_and_union/NegSyn_06020502_option_and_union_001.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060206_anytype/NegSem_060206_anytype_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_026.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_027.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_028.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSyn_060207_arrays_004.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060212_addressing_entities_inside_sut/NegSem_060212_AddressingEntitiesInsideSut_002.ttcn`: `reject -> pass`
- ... `16` more in `current-misses.json`

### `core_language/22_communication_operations`

- `core_language/22_communication_operations/2202_message_based_communication/220201_send_operation/NegSem_220201_SendOperation_005.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220201_send_operation/NegSem_220201_SendOperation_011.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220202_receive_operation/NegSem_220202_ReceiveOperation_006.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220202_receive_operation/NegSem_220202_ReceiveOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220203_trigger_operation/NegSem_220203_TriggerOperation_006.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220203_trigger_operation/NegSem_220203_TriggerOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220301_call_operation/NegSem_220301_CallOperation_012.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220301_call_operation/NegSem_220301_CallOperation_020.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220302_getcall_operation/NegSem_220302_getcall_operation_012.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220302_getcall_operation/NegSem_220302_getcall_operation_017.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220303_reply_operation/NegSem_220303_ReplyOperation_008.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220303_reply_operation/NegSem_220303_ReplyOperation_010.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_014.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_016.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_020.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220305_raise_operation/NegSem_220305_raise_operation_004.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220305_raise_operation/NegSem_220305_raise_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220306_catch_operation/NegSem_220306_catch_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220306_catch_operation/NegSem_220306_catch_operation_014.ttcn`: `reject -> pass`
- ... `2` more in `current-misses.json`

### `core_language/05_basic_language_elements`

- `core_language/05_basic_language_elements/0503_ordering_of_declarations/NegSem_0503_Ordering_002.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0503_ordering_of_declarations/NegSem_0503_Ordering_003.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040101_parameters_of_kind_value/NegSem_05040101_parameters_of_kind_value_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_020.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_021.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSyn_05040102_parameters_of_kind_template_001.ttcn`: ` -> `
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040103_parameters_of_kind_timer/NegSyn_05040103_parameters_of_kind_timer_001.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040103_parameters_of_kind_timer/NegSyn_05040103_parameters_of_kind_timer_002.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040104_parameters_of_kind_port/NegSem_05040104_parameters_of_kind_port_003.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040104_parameters_of_kind_port/NegSem_05040104_parameters_of_kind_port_004.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_221.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_223.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_224.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_225.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_229.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_230.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/0504_toplevel/NegSyn_0504_forbidden_parametrization_001.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0505_cyclic_definitions/NegSem_0505_cyclic_definitions_001.ttcn`: ` -> `
- `core_language/05_basic_language_elements/05_toplevel/NegSyn_05_TopLevel_001.ttcn`: ` -> `

### `core_language/15_templates`

- `core_language/15_templates/1503_global_and_local_templates/NegSyn_1503_GlobalAndLocalTemplates_006.ttcn`: ` -> `
- `core_language/15_templates/1503_global_and_local_templates/Sem_1503_GlobalAndLocalTemplates_010.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_010.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_011.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_012.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_013.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_015.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150605_Referencing_union_alternatives/Sem_150605_Referencing_union_alternatives_002.ttcn`: `pass -> fail` - undefined value
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_055.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_056.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_057.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_058.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_059.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_060.ttcn`: `reject -> pass`
- `core_language/15_templates/1508_template_restrictions/NegSem_1508_TemplateRestrictions_061.ttcn`: `reject -> pass`
- `core_language/15_templates/1511_concatenating_templates_of_string_and_list_types/NegSem_1511_ConcatenatingTemplatesOfStringAndListTypes_004.ttcn`: `reject -> pass`
- `core_language/15_templates/1511_concatenating_templates_of_string_and_list_types/Sem_1511_ConcatenatingTemplatesOfStringAndListTypes_013.ttcn`: `pass -> fail`
- `core_language/15_templates/15_toplevel/NegSyn_15_TopLevel_001.ttcn`: ` -> `

### `core_language/16_functions_altsteps_testcases`

- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/NegSem_160102_predefined_functions_009.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/NegSem_160102_predefined_functions_039.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/Sem_160102_predefined_functions_107.ttcn`: `pass -> fail` - 1 undefined value
- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/Sem_160102_predefined_functions_110.ttcn`: `pass -> fail` - '00'O
- `core_language/16_functions_altsteps_testcases/1601_functions/160103_external_functions/NegSem_160103_external_functions_001.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/160103_external_functions/Sem_160103_external_functions_001.ttcn`: `pass -> fail`
- `core_language/16_functions_altsteps_testcases/1601_functions/160103_external_functions/Sem_160103_external_functions_002.ttcn`: `pass -> fail`
- `core_language/16_functions_altsteps_testcases/1601_functions/160105_explicit_control_functions/NegSem_160105_explicit_control_functions_009.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/1601_toplevel/NegSem_1601_toplevel_001.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1602_altsteps/160201_invoking_altsteps/NegSyn_160201_invoking_altsteps_001.ttcn`: ` -> `
