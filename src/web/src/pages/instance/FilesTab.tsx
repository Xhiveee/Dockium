import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { DragEvent as ReactDragEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronRight,
  Download,
  File as FileIcon,
  FileArchive,
  FileCode2,
  FileImage,
  FileText,
  Folder,
  FolderPlus,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { downloadToBrowser, decodeUtf8, encodeUtf8, uploadFile } from '../../api/streams'
import { useSettingsQuery } from '../../data/queries'
import type { FileEntry, Instance } from '../../gen/panel/v1/panel_pb'
import { baseName, extensionOf, joinPath, parentPath } from '../../lib/paths'
import { formatBytes, formatDate, formatMode } from '../../lib/format'
import { cx } from '../../lib/cx'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { Drawer } from '../../ui/Dialog'
import { Field, Input, Textarea } from '../../ui/Field'
import { EmptyState, InlineNotice, Skeleton } from '../../ui/Display'
import { Menu, type MenuItem } from '../../ui/Menu'
import { useToast } from '../../ui/Toast'
import styles from './FilesTab.module.css'

type EditorState = {
  path: string
  name: string
  content: string
  original: string
  size: number
  binary: boolean
  loading: boolean
  error: string
}

type ChmodTarget = { path: string; name: string; mode: string }

function iconFor(entry: FileEntry) {
  if (entry.isDir) return Folder
  const extension = extensionOf(entry.name)
  if (['zip', 'tar', 'gz', 'tgz', '7z', 'rar'].includes(extension)) return FileArchive
  if (['png', 'jpg', 'jpeg', 'webp', 'gif', 'svg', 'ico'].includes(extension)) return FileImage
  if (['js', 'ts', 'tsx', 'jsx', 'go', 'py', 'sh', 'json', 'yml', 'yaml', 'toml', 'css', 'html'].includes(extension)) {
    return FileCode2
  }
  if (['txt', 'log', 'md', 'conf', 'cfg', 'ini', 'properties', 'xml'].includes(extension)) return FileText
  return FileIcon
}

function hasFiles(event: ReactDragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes('Files')
}

export function FilesTab({ instance }: { instance: Instance }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const settingsQuery = useSettingsQuery()
  const [path, setPath] = useState('')
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [closeConfirm, setCloseConfirm] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<FileEntry | null>(null)
  const [chmodTarget, setChmodTarget] = useState<ChmodTarget | null>(null)
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [mkdirValue, setMkdirValue] = useState('')
  const [newFileOpen, setNewFileOpen] = useState(false)
  const [newFileValue, setNewFileValue] = useState('')
  const [working, setWorking] = useState(false)
  const [dragDepth, setDragDepth] = useState(0)
  const [uploading, setUploading] = useState<{
    done: number
    total: number
    current: string
    uploadedBytes: number
    totalBytes: number
  } | null>(null)
  const [conflictFiles, setConflictFiles] = useState<File[] | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const fileKey = useMemo(() => ['files', instance.id.toString(), path] as const, [instance.id, path])

  const filesQuery = useQuery({
    queryKey: fileKey,
    queryFn: () => api.files.listFiles({ instanceId: instance.id, path }),
    staleTime: 10_000,
  })

  const entries = filesQuery.data?.items ?? []
  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: fileKey })
  }, [queryClient, fileKey])

  useEffect(() => {
    const prevent = (event: DragEvent) => {
      if (event.dataTransfer && Array.from(event.dataTransfer.types).includes('Files')) {
        event.preventDefault()
      }
    }
    const onDrop = (event: DragEvent) => {
      if (event.dataTransfer && Array.from(event.dataTransfer.types).includes('Files')) {
        event.preventDefault()
      }
    }
    window.addEventListener('dragover', prevent)
    window.addEventListener('drop', onDrop)
    return () => {
      window.removeEventListener('dragover', prevent)
      window.removeEventListener('drop', onDrop)
    }
  }, [])

  const uploadFiles = async (fileList: File[], overwrite: boolean) => {
    if (!fileList.length) return
    const maxUploadMB = settingsQuery.data?.maxUploadMb ?? 1024
    const maxUploadBytes = maxUploadMB * 1024 * 1024
    setUploading({
      done: 0,
      total: fileList.length,
      current: fileList[0]?.name ?? '',
      uploadedBytes: 0,
      totalBytes: fileList[0]?.size ?? 0,
    })
    try {
      for (let index = 0; index < fileList.length; index += 1) {
        const file = fileList[index]
        if (!file) continue
        setUploading({
          done: index,
          total: fileList.length,
          current: file.name,
          uploadedBytes: 0,
          totalBytes: file.size,
        })
        try {
          await uploadFile(
            instance.id,
            joinPath(path, file.name),
            file,
            overwrite,
            maxUploadBytes,
            (uploadedBytes, totalBytes) => {
              setUploading({
                done: index,
                total: fileList.length,
                current: file.name,
                uploadedBytes,
                totalBytes,
              })
            },
          )
        } catch (error) {
          if (!overwrite && errorMessage(error).includes('существует')) {
            setConflictFiles(fileList.slice(index))
            return
          }
          throw error
        }
      }
      toast.success(overwrite ? 'Файлы заменены' : 'Файлы загружены', fileList.length + ' шт.')
      await refresh()
    } catch (error) {
      toast.error('Не удалось загрузить файлы', errorMessage(error))
    } finally {
      setUploading(null)
    }
  }

  const handleDrop = (event: ReactDragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setDragDepth(0)
    const files = Array.from(event.dataTransfer.files)
    if (files.length) void uploadFiles(files, false)
  }

  const openEditor = async (entry: FileEntry) => {
    setEditor({
      path: entry.path,
      name: entry.name,
      content: '',
      original: '',
      size: Number(entry.size),
      binary: false,
      loading: true,
      error: '',
    })
    try {
      const file = await api.files.readFile({ instanceId: instance.id, path: entry.path })
      if (file.binary) {
        setEditor({
          path: entry.path,
          name: entry.name,
          content: '',
          original: '',
          size: Number(file.size),
          binary: true,
          loading: false,
          error: '',
        })
        return
      }
      const content = decodeUtf8(file.content)
      setEditor({
        path: entry.path,
        name: entry.name,
        content,
        original: content,
        size: Number(file.size),
        binary: false,
        loading: false,
        error: '',
      })
    } catch (error) {
      setEditor((current) => (current ? { ...current, loading: false, error: errorMessage(error) } : null))
    }
  }

  const closeEditor = () => {
    if (!editor) return
    if (!editor.binary && editor.content !== editor.original) {
      setCloseConfirm(true)
      return
    }
    setEditor(null)
  }

  const saveEditor = async () => {
    if (!editor || editor.binary) return
    setWorking(true)
    setEditor({ ...editor, loading: true, error: '' })
    try {
      await api.files.writeFile({
        instanceId: instance.id,
        path: editor.path,
        content: encodeUtf8(editor.content),
        create: false,
      })
      setEditor({ ...editor, original: editor.content, loading: false, size: encodeUtf8(editor.content).length })
      toast.success('Файл сохранён', editor.name)
      await refresh()
    } catch (error) {
      const message = errorMessage(error)
      setEditor({ ...editor, loading: false, error: message })
      toast.error('Не удалось сохранить файл', message)
    } finally {
      setWorking(false)
    }
  }

  const createDirectory = async () => {
    const name = mkdirValue.trim()
    if (!name) return
    setWorking(true)
    try {
      await api.files.createDirectory({ instanceId: instance.id, path: joinPath(path, name) })
      toast.success('Каталог создан', name)
      setMkdirOpen(false)
      setMkdirValue('')
      await refresh()
    } catch (error) {
      toast.error('Не удалось создать каталог', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const createFile = async () => {
    const name = newFileValue.trim()
    if (!name) return
    setWorking(true)
    try {
      await api.files.writeFile({
        instanceId: instance.id,
        path: joinPath(path, name),
        content: new Uint8Array(),
        create: true,
      })
      toast.success('Файл создан', name)
      setNewFileOpen(false)
      setNewFileValue('')
      await refresh()
      const created: FileEntry = {
        $typeName: 'dockium.panel.v1.FileEntry',
        name,
        path: joinPath(path, name),
        isDir: false,
        size: 0n,
        modifiedAt: BigInt(Date.now()),
        mode: 0o644,
      }
      await openEditor(created)
    } catch (error) {
      toast.error('Не удалось создать файл', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const rename = async () => {
    if (!renameTarget) return
    const name = renameValue.trim()
    if (!name) return
    setWorking(true)
    try {
      await api.files.renameFile({
        instanceId: instance.id,
        oldPath: renameTarget.path,
        newPath: joinPath(parentPath(renameTarget.path), name),
      })
      toast.success('Переименовано', `${renameTarget.name} → ${name}`)
      setRenameTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось переименовать', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    setWorking(true)
    try {
      await api.files.deleteFile({
        instanceId: instance.id,
        path: deleteTarget.path,
        recursive: deleteTarget.isDir,
      })
      toast.success('Удалено', deleteTarget.name)
      setDeleteTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось удалить', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const chmod = async () => {
    if (!chmodTarget) return
    setWorking(true)
    try {
      const mode = parseInt(chmodTarget.mode, 8)
      await api.files.chmodFile({ instanceId: instance.id, path: chmodTarget.path, mode })
      toast.success('Права изменены', `${chmodTarget.name}: ${formatMode(mode)}`)
      setChmodTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось изменить права', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const compress = async (entry: FileEntry) => {
    try {
      const response = await api.files.compressFile({
        instanceId: instance.id,
        path: entry.path,
        target: `${entry.path}.zip`,
        overwrite: false,
      })
      toast.success('Архив создан', response.path)
      await refresh()
    } catch (error) {
      toast.error('Не удалось создать архив', errorMessage(error))
    }
  }

  const download = async (entry: FileEntry) => {
    try {
      await downloadToBrowser(instance.id, entry.path, entry.name)
    } catch (error) {
      toast.error('Не удалось скачать файл', errorMessage(error))
    }
  }

  const menuItems = (entry: FileEntry): MenuItem[] => {
    const items: MenuItem[] = []
    if (!entry.isDir) {
      items.push({
        id: 'edit',
        label: 'Редактировать',
        icon: <Pencil size={14} />,
        onSelect: () => void openEditor(entry),
      })
      items.push({
        id: 'download',
        label: 'Скачать',
        icon: <Download size={14} />,
        onSelect: () => void download(entry),
      })
    }
    items.push({
      id: 'compress',
      label: 'Сжать в ZIP',
      icon: <FileArchive size={14} />,
      onSelect: () => void compress(entry),
    })
    items.push({
      id: 'rename',
      label: 'Переименовать',
      icon: <Pencil size={14} />,
      onSelect: () => {
        setRenameTarget(entry)
        setRenameValue(entry.name)
      },
    })
    items.push({
      id: 'chmod',
      label: 'Права доступа',
      onSelect: () => setChmodTarget({ path: entry.path, name: entry.name, mode: formatMode(entry.mode) }),
    })
    items.push({
      id: 'delete',
      label: 'Удалить',
      icon: <Trash2 size={14} />,
      danger: true,
      separator: true,
      onSelect: () => setDeleteTarget(entry),
    })
    return items
  }

  const segments = path ? path.split('/') : []
  const editorDirty = Boolean(editor && !editor.binary && editor.content !== editor.original)

  return (
    <div
      className={styles.wrap}
      onDragEnter={(event) => {
        if (!hasFiles(event)) return
        event.preventDefault()
        setDragDepth((depth) => depth + 1)
      }}
      onDragOver={(event) => {
        if (!hasFiles(event)) return
        event.preventDefault()
        event.dataTransfer.dropEffect = 'copy'
      }}
      onDragLeave={(event) => {
        if (!hasFiles(event)) return
        setDragDepth((depth) => Math.max(0, depth - 1))
      }}
      onDrop={handleDrop}
    >
      <div className={styles.panel}>
        <div className={styles.toolbar}>
          <nav className={styles.breadcrumbs} aria-label="Путь в файлах">
            <button type="button" className={styles.crumb} onClick={() => setPath('')}>
              Данные
            </button>
            {segments.map((segment, index) => {
              const target = segments.slice(0, index + 1).join('/')
              return (
                <span className={styles.crumbGroup} key={target}>
                  <ChevronRight size={12} className={styles.crumbSep} aria-hidden />
                  <button
                    type="button"
                    className={styles.crumb}
                    onClick={() => setPath(target)}
                    aria-current={index === segments.length - 1 ? 'page' : undefined}
                  >
                    {segment}
                  </button>
                </span>
              )
            })}
          </nav>
          <div className={styles.actions}>
            <Button
              variant="ghost"
              size="sm"
              iconOnly
              aria-label="Обновить"
              icon={<RefreshCw size={14} />}
              onClick={() => void refresh()}
              loading={filesQuery.isFetching}
            />
            <Button variant="ghost" size="sm" icon={<FolderPlus size={14} />} onClick={() => setMkdirOpen(true)}>
              Папка
            </Button>
            <Button variant="ghost" size="sm" icon={<FileText size={14} />} onClick={() => setNewFileOpen(true)}>
              Файл
            </Button>
            <Button
              size="sm"
              variant="secondary"
              icon={<Upload size={14} />}
              onClick={() => fileInputRef.current?.click()}
              loading={Boolean(uploading)}
            >
              Загрузить
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className={styles.hiddenInput}
              onChange={(event) => {
                const files = Array.from(event.target.files ?? [])
                event.target.value = ''
                if (files.length) void uploadFiles(files, false)
              }}
              aria-label="Выбрать файлы для загрузки"
            />
          </div>
        </div>

        {uploading ? (
          <div className={styles.uploadBar}>
            <span className={styles.uploadText}>
              Загрузка {Math.min(uploading.done + 1, uploading.total)} из {uploading.total}: {uploading.current}
              {' · '}
              {formatBytes(uploading.uploadedBytes)} / {formatBytes(uploading.totalBytes)}
              {' · '}
              {uploading.totalBytes > 0 ? Math.round((uploading.uploadedBytes / uploading.totalBytes) * 100) : 100}%
            </span>
            <span className={styles.uploadTrack}>
              <span
                className={styles.uploadFill}
                style={{
                  width: `${uploading.totalBytes > 0 ? Math.round((uploading.uploadedBytes / uploading.totalBytes) * 100) : 100}%`,
                }}
              />
            </span>
          </div>
        ) : null}

        {filesQuery.isPending ? (
          <div className={styles.rows}>
            {Array.from({ length: 6 }, (_, index) => (
              <div className={styles.row} key={index}>
                <Skeleton width="45%" height={12} />
                <Skeleton width={60} height={11} />
                <Skeleton width={100} height={11} />
                <Skeleton width={24} height={24} />
              </div>
            ))}
          </div>
        ) : filesQuery.isError ? (
          <div className={styles.stateWrap}>
            <InlineNotice tone="danger" title="Не удалось прочитать каталог">
              {errorMessage(filesQuery.error)}
            </InlineNotice>
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon={Folder}
            title="Каталог пуст"
            description="Перетащите файлы сюда или создайте первый файл. Данные экземпляра хранятся на агенте."
            action={<Button onClick={() => fileInputRef.current?.click()}>Загрузить файлы</Button>}
          />
        ) : (
          <div className={styles.rows} role="list">
            <div className={styles.rowsHead}>
              <span>Имя</span>
              <span>Размер</span>
              <span>Изменён</span>
              <span />
            </div>
            {entries.map((entry) => {
              const Icon = iconFor(entry)
              return (
                <div
                  key={entry.path}
                  className={styles.row}
                  role="listitem"
                  onClick={() => {
                    if (entry.isDir) setPath(entry.path)
                    else void openEditor(entry)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      if (entry.isDir) setPath(entry.path)
                      else void openEditor(entry)
                    }
                  }}
                  tabIndex={0}
                >
                  <span className={styles.nameCell}>
                    <Icon size={15} className={cx(styles.fileIcon, entry.isDir && styles.dirIcon)} aria-hidden />
                    <span className={styles.nameText}>{entry.name}</span>
                  </span>
                  <span className={styles.sizeCell}>{entry.isDir ? '—' : formatBytes(entry.size)}</span>
                  <span className={styles.dateCell}>{formatDate(entry.modifiedAt, true)}</span>
                  <span className={styles.rowActions} onClick={(event) => event.stopPropagation()}>
                    <Menu items={menuItems(entry)} label={`Действия: ${entry.name}`} />
                  </span>
                </div>
              )
            })}
          </div>
        )}

        <footer className={styles.footer}>
          <span>{entries.length} объектов</span>
          <span className={styles.footerHint}>
            Перетащите файлы в эту панель, чтобы загрузить · до {settingsQuery.data?.maxUploadMb ?? 1024} МБ
          </span>
        </footer>

        {dragDepth > 0 ? (
          <div className={styles.dropOverlay} aria-hidden>
            <div className={styles.dropBox}>
              <Upload size={18} />
              <span>Отпустите файлы, чтобы загрузить в «{path ? baseName(path) : 'Данные'}»</span>
            </div>
          </div>
        ) : null}
      </div>

      <Drawer
        open={Boolean(editor)}
        onClose={closeEditor}
        width={760}
        title={editor ? editor.name : ''}
        description={
          editor
            ? `${editor.path} · ${formatBytes(editor.size)}${editorDirty ? ' · не сохранено' : ''}`
            : undefined
        }
        dismissible={!working}
        footer={
          editor ? (
            <>
              <Button variant="ghost" onClick={closeEditor} disabled={working}>Закрыть</Button>
              {!editor.binary ? (
                <Button
                  variant="primary"
                  onClick={() => void saveEditor()}
                  loading={working || editor.loading}
                  disabled={!editorDirty}
                >
                  Сохранить
                </Button>
              ) : null}
            </>
          ) : null
        }
      >
        {editor ? (
          <div className={styles.editor}>
            {editor.error ? <InlineNotice tone="danger" title={editor.error} /> : null}
            {editor.binary ? (
              <InlineNotice tone="info" title="Бинарный файл">
                Предпросмотр недоступен. Скачайте файл, чтобы открыть его локально.
              </InlineNotice>
            ) : editor.loading && !editor.content ? (
              <div className={styles.editorLoading}>
                <Skeleton width="90%" height={12} />
                <Skeleton width="80%" height={12} />
                <Skeleton width="60%" height={12} />
              </div>
            ) : (
              <Textarea
                className={styles.editorArea}
                mono
                spellCheck={false}
                value={editor.content}
                onChange={(event) => setEditor({ ...editor, content: event.target.value })}
                aria-label={`Содержимое ${editor.name}`}
              />
            )}
            {editor.binary ? (
              <Button
                variant="secondary"
                icon={<Download size={14} />}
                onClick={() => void download({
                  $typeName: 'dockium.panel.v1.FileEntry',
                  name: editor.name,
                  path: editor.path,
                  isDir: false,
                  size: BigInt(editor.size),
                  modifiedAt: 0n,
                  mode: 0,
                })}
              >
                Скачать файл
              </Button>
            ) : null}
          </div>
        ) : null}
      </Drawer>

      <Dialog
        open={mkdirOpen}
        onClose={() => setMkdirOpen(false)}
        title="Новый каталог"
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setMkdirOpen(false)} disabled={working}>Отмена</Button>
            <Button variant="primary" onClick={() => void createDirectory()} loading={working} disabled={!mkdirValue.trim()}>
              Создать
            </Button>
          </>
        }
      >
        <Field label="Имя каталога" htmlFor="mkdir-name" hint={`Будет создан в «${path || 'Данные'}»`}>
          <Input
            id="mkdir-name"
            value={mkdirValue}
            onChange={(event) => setMkdirValue(event.target.value)}
            placeholder="plugins"
            autoFocus
            maxLength={100}
          />
        </Field>
      </Dialog>

      <Dialog
        open={newFileOpen}
        onClose={() => setNewFileOpen(false)}
        title="Новый файл"
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewFileOpen(false)} disabled={working}>Отмена</Button>
            <Button variant="primary" onClick={() => void createFile()} loading={working} disabled={!newFileValue.trim()}>
              Создать
            </Button>
          </>
        }
      >
        <Field label="Имя файла" htmlFor="newfile-name" hint="Файл откроется в редакторе.">
          <Input
            id="newfile-name"
            value={newFileValue}
            onChange={(event) => setNewFileValue(event.target.value)}
            placeholder="server.properties"
            autoFocus
            maxLength={100}
          />
        </Field>
      </Dialog>

      <Dialog
        open={Boolean(renameTarget)}
        onClose={() => setRenameTarget(null)}
        title="Переименовать"
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRenameTarget(null)} disabled={working}>Отмена</Button>
            <Button variant="primary" onClick={() => void rename()} loading={working} disabled={!renameValue.trim()}>
              Переименовать
            </Button>
          </>
        }
      >
        <Field label="Новое имя" htmlFor="rename-name">
          <Input
            id="rename-name"
            value={renameValue}
            onChange={(event) => setRenameValue(event.target.value)}
            autoFocus
            maxLength={100}
          />
        </Field>
      </Dialog>

      <Dialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        title="Удалить?"
        description={deleteTarget ? deleteTarget.path : undefined}
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={working}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void remove()} loading={working}>Удалить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          {deleteTarget?.isDir
            ? 'Каталог будет удалён вместе со всем содержимым. Восстановить данные будет нельзя.'
            : 'Файл будет удалён без возможности восстановления.'}
        </p>
      </Dialog>

      <Dialog
        open={Boolean(chmodTarget)}
        onClose={() => setChmodTarget(null)}
        title="Права доступа"
        description={chmodTarget ? chmodTarget.path : undefined}
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setChmodTarget(null)} disabled={working}>Отмена</Button>
            <Button
              variant="primary"
              onClick={() => void chmod()}
              loading={working}
              disabled={!chmodTarget || !/^[0-7]{3,4}$/.test(chmodTarget.mode)}
            >
              Применить
            </Button>
          </>
        }
      >
        <Field label="Режим (восьмеричное значение)" htmlFor="chmod-mode" hint="Например, 644 или 755">
          <Input
            id="chmod-mode"
            value={chmodTarget?.mode ?? ''}
            onChange={(event) => setChmodTarget((current) => (current ? { ...current, mode: event.target.value } : current))}
            mono
            maxLength={4}
            autoFocus
          />
        </Field>
      </Dialog>

      <Dialog
        open={Boolean(conflictFiles)}
        onClose={() => setConflictFiles(null)}
        title="Файлы уже существуют"
        size="sm"
        dismissible={!uploading}
        footer={
          <>
            <Button variant="ghost" onClick={() => setConflictFiles(null)} disabled={Boolean(uploading)}>Отмена</Button>
            <Button
              variant="primary"
              loading={Boolean(uploading)}
              onClick={() => {
                const files = conflictFiles
                setConflictFiles(null)
                if (files) void uploadFiles(files, true)
              }}
            >
              Заменить файлы
            </Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          В каталоге уже есть файлы с такими именами. Заменить их новыми?
        </p>
      </Dialog>

      <Dialog
        open={closeConfirm}
        onClose={() => setCloseConfirm(false)}
        title="Есть несохранённые изменения"
        description="Закрыть редактор без сохранения?"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setCloseConfirm(false)}>Продолжить редактирование</Button>
            <Button
              variant="dangerSolid"
              onClick={() => {
                setCloseConfirm(false)
                setEditor(null)
              }}
            >
              Закрыть без сохранения
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  )
}
