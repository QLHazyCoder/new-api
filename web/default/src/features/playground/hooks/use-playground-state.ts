/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef, useState } from 'react'

import {
  DEFAULT_CONFIG,
  DEFAULT_IMAGE_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '../constants'
import {
  applyMessageStateUpdate,
  getInitialParameterEnabled,
  getInitialPlaygroundConfig,
  loadImageConfig,
  loadImageTasks,
  loadMessages,
  loadPlaygroundMode,
  persistInterruptedImageTasks,
  saveConfig,
  saveImageConfig,
  saveImageTasks,
  saveMessages,
  saveParameterEnabled,
  savePlaygroundMode,
  type MessageStateUpdater,
} from '../lib'
import type {
  GroupOption,
  ImageGenerationConfig,
  ImageTask,
  Message,
  ModelOption,
  ParameterEnabled,
  PlaygroundConfig,
  PlaygroundMode,
} from '../types'

const MESSAGE_SAVE_DEBOUNCE_MS = 500

/**
 * Main state management hook for playground
 */
export function usePlaygroundState() {
  const [mode, setModeState] = useState<PlaygroundMode>(() => {
    return loadPlaygroundMode()
  })

  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(
    getInitialPlaygroundConfig
  )
  const [imageConfig, setImageConfig] = useState<ImageGenerationConfig>(() => {
    const savedConfig = loadImageConfig()
    return { ...DEFAULT_IMAGE_CONFIG, ...savedConfig }
  })
  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    getInitialParameterEnabled
  )

  const [messages, setMessages] = useState<Message[]>([])
  const [isLoadingMessages, setIsLoadingMessages] = useState(true)
  const messagesSaveTimerRef = useRef<number | null>(null)
  const latestMessagesRef = useRef<Message[]>(messages)
  const hasLoadedMessagesRef = useRef(false)

  const [imageTasks, setImageTasks] = useState<ImageTask[]>(() => {
    return loadImageTasks()
  })
  const imageTasksRef = useRef(imageTasks)

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])

  useEffect(() => {
    imageTasksRef.current = imageTasks
  }, [imageTasks])

  useEffect(() => {
    const handlePageExit = () => {
      persistInterruptedImageTasks(imageTasksRef.current)
    }

    window.addEventListener('pagehide', handlePageExit)
    window.addEventListener('beforeunload', handlePageExit)

    return () => {
      window.removeEventListener('pagehide', handlePageExit)
      window.removeEventListener('beforeunload', handlePageExit)
    }
  }, [])

  const persistMessages = useCallback((messagesToSave: Message[]) => {
    latestMessagesRef.current = messagesToSave

    if (!hasLoadedMessagesRef.current) {
      return
    }

    if (messagesSaveTimerRef.current !== null) {
      window.clearTimeout(messagesSaveTimerRef.current)
    }

    messagesSaveTimerRef.current = window.setTimeout(() => {
      messagesSaveTimerRef.current = null
      saveMessages(latestMessagesRef.current)
    }, MESSAGE_SAVE_DEBOUNCE_MS)
  }, [])

  useEffect(() => {
    let cancelled = false

    window.setTimeout(() => {
      const loadedMessages = loadMessages() ?? []
      if (cancelled) {
        return
      }

      latestMessagesRef.current = loadedMessages
      hasLoadedMessagesRef.current = true
      setMessages(loadedMessages)
      setIsLoadingMessages(false)
    }, 0)

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(
    () => () => {
      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
        saveMessages(latestMessagesRef.current)
      }
    },
    []
  )

  // Update config with automatic save
  const setMode = useCallback((value: PlaygroundMode) => {
    setModeState(value)
    savePlaygroundMode(value)
  }, [])

  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated)
        return updated
      })
    },
    []
  )

  const updateImageConfig = useCallback(
    <K extends keyof ImageGenerationConfig>(
      key: K,
      value: ImageGenerationConfig[K]
    ) => {
      setImageConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveImageConfig(updated)
        return updated
      })
    },
    []
  )

  const replaceImageConfig = useCallback((value: ImageGenerationConfig) => {
    setImageConfig(value)
    saveImageConfig(value)
  }, [])

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: MessageStateUpdater) => {
      setMessages((prev) => {
        const newMessages = applyMessageStateUpdate(prev, updater)
        persistMessages(newMessages)
        return newMessages
      })
    },
    [persistMessages]
  )

  const updateImageTasks = useCallback(
    (updater: ImageTask[] | ((prev: ImageTask[]) => ImageTask[])) => {
      setImageTasks((prev) => {
        const newTasks = typeof updater === 'function' ? updater(prev) : updater
        saveImageTasks(newTasks)
        return newTasks
      })
    },
    []
  )

  // Clear all messages
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setImageConfig(DEFAULT_IMAGE_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveImageConfig(DEFAULT_IMAGE_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
  }, [])

  return {
    // State
    mode,
    config,
    imageConfig,
    parameterEnabled,
    messages,
    isLoadingMessages,
    imageTasks,
    models,
    groups,

    // Setters
    setModels,
    setGroups,

    // Actions
    setMode,
    updateConfig,
    updateImageConfig,
    replaceImageConfig,
    updateParameterEnabled,
    updateMessages,
    updateImageTasks,
    clearMessages,
    resetConfig,
  }
}
