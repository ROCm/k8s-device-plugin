sysfs data capture from a physical node with 1 GPU

node 2 is cp of 1 to simulate a multi GPU system

```
find /sys/class/kfd/kfd/topology/ -type d -exec sh -c 'mkdir -p .$1' sh {} \;
find /sys/class/kfd/kfd/topology/ -type f -exec sh -c 'cat $1 > .$1' sh {} \;
```
