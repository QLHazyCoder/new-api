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
import { useEffect, useMemo, useState } from 'react'

import { PlaygroundChat } from './components/chat/playground-chat'
import { PlaygroundInput } from './components/input/playground-input'
import { PlaygroundImageInput } from './components/playground-image-input'
import { PlaygroundImageTaskGrid } from './components/playground-image-task-grid'
import { PlaygroundModeToggle } from './components/playground-mode-toggle'
import {
  useChatHandler,
  useImageGenerationHandler,
  usePlaygroundConversation,
  usePlaygroundImageOptions,
  usePlaygroundOptions,
  usePlaygroundState,
} from './hooks'
import {
  EMPTY_IMAGE_MODEL_CAPABILITIES,
  imageConfigsEqual,
  normalizePlaygroundImageConfig,
  resolveImageModelSelection,
} from './lib'
import type { ImageTask } from './types'

export function Playground() {
  const {
    mode,
    config,
    imageConfig,
    parameterEnabled,
    messages,
    isLoadingMessages,
    imageTasks,
    models,
    groups,
    setMode,
    updateMessages,
    updateImageTasks,
    setModels,
    setGroups,
    updateConfig,
    updateImageConfig,
    replaceImageConfig,
    updateParameterEnabled,
    clearMessages,
  } = usePlaygroundState()

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

  const {
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  } = usePlaygroundConversation({
    messages,
    updateMessages,
    sendChat,
  })

  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })

  const { imageGroups, isLoadingImageOptions } = usePlaygroundImageOptions()
  const imageSelection = useMemo(
    () =>
      resolveImageModelSelection(
        imageGroups,
        imageConfig.group,
        imageConfig.model
      ),
    [imageConfig.group, imageConfig.model, imageGroups]
  )
  const effectiveImageConfig = useMemo(() => {
    if (!imageSelection) return imageConfig
    return normalizePlaygroundImageConfig(
      {
        ...imageConfig,
        group: imageSelection.group.value,
        model: imageSelection.model.value,
      },
      imageSelection.model.capabilities
    )
  }, [imageConfig, imageSelection])
  const imageModels = imageSelection?.group.models ?? []
  const imageCapabilities =
    imageSelection?.model.capabilities ?? EMPTY_IMAGE_MODEL_CAPABILITIES
  const [imagePrompt, setImagePrompt] = useState('')

  const { generateImage, retryTask } = useImageGenerationHandler({
    config: effectiveImageConfig,
    groups: imageGroups,
    onTasksUpdate: updateImageTasks,
  })

  useEffect(() => {
    if (!imageSelection) return
    if (!imageConfigsEqual(imageConfig, effectiveImageConfig)) {
      replaceImageConfig(effectiveImageConfig)
    }
  }, [effectiveImageConfig, imageConfig, imageSelection, replaceImageConfig])

  const handleImageGroupChange = (value: string) => {
    const group = imageGroups.find((option) => option.value === value)
    if (!group) return
    const model =
      group.models.find(
        (option) => option.value === effectiveImageConfig.model
      ) ?? group.models[0]
    if (!model) return
    replaceImageConfig(
      normalizePlaygroundImageConfig(
        { ...effectiveImageConfig, group: group.value, model: model.value },
        model.capabilities
      )
    )
  }

  const handleImageModelChange = (value: string) => {
    const model = imageModels.find((option) => option.value === value)
    if (!model) return
    replaceImageConfig(
      normalizePlaygroundImageConfig(
        { ...effectiveImageConfig, model: model.value },
        model.capabilities
      )
    )
  }

  const handleReusePrompt = (prompt: string) => {
    setMode('image')
    setImagePrompt(prompt)
  }

  const handleRetryTask = (task: ImageTask) => {
    retryTask(task)
  }

  const handleDeleteImageTask = (taskId: string) => {
    updateImageTasks((prev) => prev.filter((task) => task.id !== taskId))
  }

  const handleClearMessages = () => {
    handleEditOpenChange(false)
    clearMessages()
  }

  return (
    <div className='relative flex size-full min-h-0 flex-col overflow-hidden'>
      <div className='mx-auto flex w-full max-w-4xl items-center justify-between px-3 py-2'>
        <PlaygroundModeToggle value={mode} onChange={setMode} />
      </div>

      {/* Full-width scroll container: scrolling works even over side whitespace */}
      <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
        {mode === 'chat' ? (
          <PlaygroundChat
            messages={messages}
            onRegenerateMessage={handleRegenerateMessage}
            onEditMessage={handleEditMessage}
            onDeleteMessage={handleDeleteMessage}
            onSelectPrompt={handleSendMessage}
            isGenerating={isGenerating}
            isLoadingMessages={isLoadingMessages}
            editingKey={editingMessageKey}
            onCancelEdit={handleEditOpenChange}
            onSaveEdit={(newContent) => applyEdit(newContent, false)}
            onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
          />
        ) : (
          <div className='min-h-0 flex-1 overflow-y-auto'>
            <PlaygroundImageTaskGrid
              tasks={imageTasks}
              onReusePrompt={handleReusePrompt}
              onRetryTask={handleRetryTask}
              onDeleteTask={handleDeleteImageTask}
            />
          </div>
        )}
      </div>

      {/* Input area: center content and constrain to the same container width */}
      <div className='mx-auto w-full max-w-4xl'>
        {mode === 'chat' ? (
          <PlaygroundInput
            config={config}
            disabled={isGenerating}
            groups={groups}
            groupValue={config.group}
            isGenerating={isGenerating}
            isModelLoading={isLoadingModels}
            modelValue={config.model}
            models={models}
            onGroupChange={(value) => updateConfig('group', value)}
            onConfigChange={updateConfig}
            onClearMessages={handleClearMessages}
            onModelChange={(value) => updateConfig('model', value)}
            onParameterEnabledChange={updateParameterEnabled}
            onStop={stopGeneration}
            onSubmit={handleSendMessage}
            parameterEnabled={parameterEnabled}
            hasMessages={messages.length > 0}
          />
        ) : (
          <PlaygroundImageInput
            config={effectiveImageConfig}
            capabilities={imageCapabilities}
            disabled={!imageSelection}
            groups={imageGroups}
            isModelLoading={isLoadingImageOptions}
            models={imageModels}
            prompt={imagePrompt}
            onConfigChange={updateImageConfig}
            onGroupChange={handleImageGroupChange}
            onModelChange={handleImageModelChange}
            onPromptChange={setImagePrompt}
            onSubmit={(prompt, referenceImages) =>
              void generateImage(prompt, referenceImages)
            }
          />
        )}
      </div>
    </div>
  )
}
