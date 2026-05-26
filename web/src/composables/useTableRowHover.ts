import { ref } from 'vue'

export function useTableRowHover() {
  const hoveredRow = ref<number | null>(null)

  function tableRowClass({ rowIndex }: { rowIndex: number }) {
    return hoveredRow.value === rowIndex ? 'row-hovered' : ''
  }

  function handleRowEnter({ rowIndex }: { rowIndex: number }) {
    hoveredRow.value = rowIndex
  }

  function handleRowLeave() {
    hoveredRow.value = null
  }

  return {
    hoveredRow,
    tableRowClass,
    handleRowEnter,
    handleRowLeave,
  }
}