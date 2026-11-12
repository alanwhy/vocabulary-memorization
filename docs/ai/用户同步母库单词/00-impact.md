
Impact of changing "AdminUsers" — 3 affected symbols:

frontend/src/components/admin/AdminUsers.vue
  component   AdminUsers:1

frontend/src/views/AdminView.vue
  component   AdminView:1
  file        AdminView.vue:1

Impact of changing "WordStore" — 98 affected symbols:

backend/internal/app/app.go
  interface   WordStore:41
  method      Insert:42
  method      FindByUserAndKey:43
  method      IncrementReview:44
  method      ApplyFlashcardReview:45
  method      DueFlashcards:46
  method      ListPage:47
  method      CountByUser:48
  method      ResetReviewCounts:49
  method      Delete:50
  method      DeleteByUserID:51
  method      DeleteByWordKey:52
  method      DeleteByWordKeys:53
  method      SetArchived:54
  method      TranslationSenses:55
  method      UpdateSenses:56
  method      UpdateImportantGlosses:57
  method      MarkTranslationStarted:58
  method      MarkTranslating:59
  method      ClearTranslating:60
  method      TryMarkTranslating:61
  method      FindTranslating:62
  method      FindTranslatingStale:63
  method      FindTranslatingByUser:64
  method      FindByIDs:65
  method      Stats:66

backend/internal/app/auth.go
  method      createUser:68
  method      BootstrapAdmin:41
  method      handleCreateUser:423
  method      handleDeleteUser:165

backend/internal/app/words.go
  method      handleAddWord:27
  method      tryIncrementExisting:270
  method      handleLookupWord:409
  method      handleFlashcardReview:489
  method      handleFlashcardQueue:474
  method      handleListWords:348
  method      handleResetReviewCounts:373
  method      setWordArchived:569
  method      handleArchiveWord:560
  method      handleUnarchiveWord:564
  method      translateAndSave:186
  method      spawnTranslation:116
  method      saveWordSenses:255
  method      handleSetWordImportant:642
  method      handleRetryWord:592
  method      resumeStuckTranslations:140
  method      sweepStuckTranslations:171
  method      startStuckTranslationSweeper:156
  method      handleListTranslatingWords:386
  method      handleWordStats:437

backend/internal/app/auth_test.go
  function    TestHandleDeleteUserCannotDeleteSelf:387

backend/internal/app/dictionary.go
  method      handleDeleteDictionaryEntry:173
  method      handleDeleteDictionaryBatch:192

backend/internal/app/correction.go
  method      ensureWordEnrichment:29
  method      restoreWordSenses:53
  method      handleCorrectWord:61

backend/internal/app/lifecycle.go
  method      Initialize:12

backend/internal/app/training_handlers.go
  method      prepareTrainingJob:106
  method      handleTraining:209

backend/internal/app/word_enrichment_test.go
  struct      enrichmentWords:20
  method      FindByIDs:29
  method      FindByUserAndKey:37
  method      IncrementReview:46
  method      DueFlashcards:53
  method      TryMarkTranslating:56
  method      ClearTranslating:66
  method      MarkTranslationStarted:72
  method      TranslationSenses:75
  method      UpdateSenses:80
  struct      staleSnapshotWords:292
  function    enrichmentApp:128

backend/internal/storage/words.go
  struct      WordRepository:14
  method      Insert:74
  method      FindByUserAndKey:87
  method      IncrementReview:104
  method      ApplyFlashcardReview:111
  method      DueFlashcards:120
  method      ListPage:135
  method      CountByUser:153
  method      ResetReviewCounts:165
  method      Delete:173
  method      DeleteByUserID:182
  method      DeleteByWordKey:191
  method      DeleteByWordKeys:200
  method      SetArchived:216
  method      UpdateSenses:224
  method      UpdateImportantGlosses:230
  method      MarkTranslationStarted:236
  method      MarkTranslating:242
  method      TryMarkTranslating:248
  method      FindTranslating:258
  method      FindTranslatingStale:277
  method      FindTranslatingByUser:299
  method      FindByIDs:309
  method      Stats:328
  method      TranslationSenses:453
  method      ClearTranslating:464
  function    NewWordRepository:19
