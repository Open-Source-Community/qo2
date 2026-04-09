# ~/.bashrc

# ----- Prompt -----
# \u = username, \h = hostname, \w = current directory
PS1='\u@\h:\w\$ '

# ----- Aliases -----
alias ll='ls -alF'
alias la='ls -A'
alias l='ls -CF'
alias grep='grep --color=auto'
alias cls='clear'

# ----- Safer defaults -----
alias cp='cp -i'
alias mv='mv -i'
alias rm='rm -i'

# ----- Environment -----
export EDITOR=nano # Change to vim or nvim if you prefer
export PAGER=less
export HISTSIZE=5000
export HISTCONTROL=ignoredups:erasedups

# ----- PATH additions -----
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export PATH="$HOME/bin:$PATH"

# ----- Enable bash completion (if installed) -----
if [ -f /etc/bash_completion ]; then
  . /etc/bash_completion
fi

# ----- Color support for ls -----
if [ -x /usr/bin/dircolors ]; then
  eval "$(dircolors -b)"
  alias ls='ls --color=auto'
fi
