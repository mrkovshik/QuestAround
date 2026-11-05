<script lang="ts">
  import QRCode from 'qrcode'

  let { text }: { text: string } = $props()
  let svg = $state('')

  $effect(() => {
    QRCode.toString(text, { type: 'svg', margin: 1, errorCorrectionLevel: 'M', color: { dark: '#000000', light: '#ffffff' } }).then(
      (s) => (svg = s),
    )
  })
</script>

<div class="qr">{@html svg}</div>

<style>
  .qr {
    background: #fff;
    border-radius: 12px;
    padding: 0.75rem;
    max-width: 280px;
    margin: 0 auto;
  }
  .qr :global(svg) {
    display: block;
    width: 100%;
    height: auto;
  }
</style>
