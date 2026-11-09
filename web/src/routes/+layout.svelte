<script>
  import '../app.css';
  import { ModeWatcher } from 'mode-watcher';

  let { children } = $props();
</script>

<ModeWatcher themeColors={{ dark: '#000000', light: '#f2f2f7' }} />

{@render children()}
