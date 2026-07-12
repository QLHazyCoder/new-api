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
  usePlaygroundImageModels,
  usePlaygroundOptions,
  usePlaygroundState,
} from './hooks'
import {
  isSupportedPlaygroundImageModel,
  supportsImageEditingModel,
} from './lib'
import type { ImageTask, ModelOption } from './types'

function supportsImageGeneration(model: ModelOption): boolean {
  const endpoints =
    model.supportedEndpointTypes || model.supported_endpoint_types
  return (
    isSupportedPlaygroundImageModel(model.value) &&
    (endpoints?.includes('image-generation') ?? false)
  )
}

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

  const { imageModelOptions, isLoadingImageModels } = usePlaygroundImageModels(
    imageConfig.group
  )

  const imageModels = useMemo(
    () => imageModelOptions.filter(supportsImageGeneration),
    [imageModelOptions]
  )
  const [imagePrompt, setImagePrompt] = useState('')

  const { generateImage, retryTask } = useImageGenerationHandler({
    config: imageConfig,
    onTasksUpdate: updateImageTasks,
  })

  useEffect(() => {
    if (imageModels.length === 0) return
    const hasCurrentImageModel = imageModels.some(
      (model) => model.value === imageConfig.model
    )
    if (!hasCurrentImageModel) {
      updateImageConfig('model', imageModels[0].value)
    }
  }, [imageModels, imageConfig.model, updateImageConfig])

  useEffect(() => {
    if (groups.length === 0) return
    const hasCurrentImageGroup = groups.some(
      (group) => group.value === imageConfig.group
    )
    if (!hasCurrentImageGroup) {
      const fallback =
        groups.find((group) => group.value === 'default')?.value ??
        groups[0].value
      updateImageConfig('group', fallback)
    }
  }, [groups, imageConfig.group, updateImageConfig])

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
            config={imageConfig}
            disabled={imageModels.length === 0}
            groups={groups}
            isModelLoading={isLoadingImageModels}
            models={imageModels}
            prompt={imagePrompt}
            supportsReferenceImages={supportsImageEditingModel(
              imageConfig.model
            )}
            onConfigChange={updateImageConfig}
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
